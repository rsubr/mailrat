package accounts

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"golang.org/x/oauth2"

	"github.com/mailrat/mailrat/internal/storage"
)

// Manager handles OAuth accounts.
type Manager struct {
	store       *storage.Store
	credentials []byte // raw credentials JSON
}

// NewManager creates an AccountManager. credentialsPath is the path to
// the Google OAuth2 credentials.json file downloaded from the Cloud Console.
func NewManager(store *storage.Store, credentialsPath string) (*Manager, error) {
	b, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}
	return &Manager{store: store, credentials: b}, nil
}

// AddAccount runs the OAuth flow, fetches the user's email, and persists
// the account to storage.
func (m *Manager) AddAccount(ctx context.Context) (storage.Account, error) {
	cfg, err := oauthConfig(m.credentials)
	if err != nil {
		return storage.Account{}, err
	}

	token, err := runOAuthFlow(cfg)
	if err != nil {
		return storage.Account{}, err
	}

	client := cfg.Client(ctx, token)
	email, err := fetchEmail(ctx, client)
	if err != nil {
		return storage.Account{}, fmt.Errorf("fetch email: %w", err)
	}

	tokenJSON, err := tokenToJSON(token)
	if err != nil {
		return storage.Account{}, err
	}

	// Use md5 of email as stable ID.
	id := fmt.Sprintf("%x", md5.Sum([]byte(email)))

	acc := storage.Account{
		ID:        id,
		Email:     email,
		TokenJSON: tokenJSON,
		HistoryID: "",
	}
	if err := m.store.UpsertAccount(acc); err != nil {
		return storage.Account{}, fmt.Errorf("save account: %w", err)
	}
	return acc, nil
}

// ListAccounts returns all stored accounts.
func (m *Manager) ListAccounts() ([]storage.Account, error) {
	return m.store.ListAccounts()
}

// GetClient returns an authenticated HTTP client for the given account ID.
// The token is refreshed automatically by the oauth2 transport.
func (m *Manager) GetClient(ctx context.Context, accountID string) (*http.Client, error) {
	acc, err := m.store.GetAccount(accountID)
	if err != nil {
		return nil, err
	}

	token, err := tokenFromJSON(acc.TokenJSON)
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	cfg, err := oauthConfig(m.credentials)
	if err != nil {
		return nil, err
	}

	src := &savingTokenSource{
		base:      cfg.TokenSource(ctx, token),
		store:     m.store,
		accountID: accountID,
	}

	return oauth2.NewClient(ctx, src), nil
}

// savingTokenSource persists refreshed tokens back to storage.
type savingTokenSource struct {
	base      oauth2.TokenSource
	store     *storage.Store
	accountID string
}

func (s *savingTokenSource) Token() (*oauth2.Token, error) {
	token, err := s.base.Token()
	if err != nil {
		return nil, err
	}
	tokenJSON, err := tokenToJSON(token)
	if err == nil {
		acc, aerr := s.store.GetAccount(s.accountID)
		if aerr == nil {
			acc.TokenJSON = tokenJSON
			_ = s.store.UpsertAccount(acc)
		}
	}
	return token, nil
}

// fetchEmail calls Google's userinfo endpoint to get the authenticated email.
func fetchEmail(ctx context.Context, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://www.googleapis.com/oauth2/v3/userinfo", nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var info struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	if info.Email == "" {
		return "", fmt.Errorf("empty email in userinfo response")
	}
	return info.Email, nil
}
