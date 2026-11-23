package accounts

import (
	"context"
	"crypto/md5"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"

	"github.com/mailrat/mailrat/internal/storage"
)

// requestTimeout bounds one API call end to end. Without it a connection that
// died while the machine slept blocks its request (and the sync holding the
// sync lock) until the OS gives up, which can take many minutes or never.
const requestTimeout = 2 * time.Minute

// transport fails fast on dead connections and drops idle ones quickly, so a
// pooled connection that went stale during sleep is not reused after wake.
var transport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 15 * time.Second,
	}).DialContext,
	ForceAttemptHTTP2:     true,
	TLSHandshakeTimeout:   15 * time.Second,
	ResponseHeaderTimeout: 60 * time.Second,
	IdleConnTimeout:       30 * time.Second,
	MaxIdleConnsPerHost:   10,
}

// CloseIdleConnections discards pooled connections; call it after the system
// resumes from sleep.
func CloseIdleConnections() { transport.CloseIdleConnections() }

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

// ValidateCredentials reports whether data is a usable Google OAuth client file.
func ValidateCredentials(data []byte) error {
	_, err := oauthConfig(data)
	return err
}

// saveAccount fetches the profile for a freshly issued token and persists the account.
func (m *Manager) saveAccount(ctx context.Context, cfg *oauth2.Config, token *oauth2.Token) (storage.Account, error) {
	client := cfg.Client(ctx, token)
	email, photoURL, displayName, err := fetchProfile(ctx, client)
	if err != nil {
		return storage.Account{}, fmt.Errorf("fetch profile: %w", err)
	}

	tokenJSON, err := tokenToJSON(token)
	if err != nil {
		return storage.Account{}, err
	}

	id := fmt.Sprintf("%x", md5.Sum([]byte(email)))
	if old, err := m.store.GetAccount(id); err == nil && old.DisplayName != "" {
		displayName = old.DisplayName // keep a name the user chose
	}
	acc := storage.Account{
		ID:          id,
		Email:       email,
		TokenJSON:   tokenJSON,
		HistoryID:   "",
		PhotoURL:    photoURL,
		DisplayName: displayName,
	}
	if err := m.store.UpsertAccount(acc); err != nil {
		return storage.Account{}, fmt.Errorf("save account: %w", err)
	}
	return acc, nil
}

// GetClient returns an authenticated HTTP client for the given account ID.
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

	// The oauth2 client built from this context uses our transport for both
	// API calls and token refreshes; it does not inherit a Timeout, so set it.
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Transport: transport, Timeout: requestTimeout})
	c := oauth2.NewClient(ctx, src)
	c.Timeout = requestTimeout
	return c, nil
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
