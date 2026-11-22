package cli

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mailrat/mailrat/internal/accounts"
	"github.com/mailrat/mailrat/internal/contacts"
	"github.com/mailrat/mailrat/internal/gmail"
	"github.com/mailrat/mailrat/internal/mail"
	"github.com/mailrat/mailrat/internal/storage"
	mailsync "github.com/mailrat/mailrat/internal/sync"
)

// app holds all initialised services for a CLI session.
type app struct {
	store    *storage.Store
	accounts *accounts.Manager
}

// newApp opens the database and initialises the account manager.
// Requires credentials.json to already exist.
func newApp() (*app, error) {
	dir, err := dataDir()
	if err != nil {
		return nil, err
	}

	dbPath := filepath.Join(dir, "mailrat.db")
	store, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	credPath := filepath.Join(dir, "credentials.json")
	mgr, err := accounts.NewManager(store, credPath)
	if err != nil {
		store.Close() //nolint:errcheck
		return nil, fmt.Errorf(
			"credentials not found at %s\n\nRun 'mailrat accounts add' to set up your account.",
			credPath,
		)
	}

	return &app{store: store, accounts: mgr}, nil
}

// newAppWithCredentials opens the database and initialises the account manager
// using the provided credentials path. Used by 'accounts add' after the user
// has supplied their credentials.json.
func newAppWithCredentials(credPath string) (*app, error) {
	dir, err := dataDir()
	if err != nil {
		return nil, err
	}

	dbPath := filepath.Join(dir, "mailrat.db")
	store, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	mgr, err := accounts.NewManager(store, credPath)
	if err != nil {
		store.Close() //nolint:errcheck
		return nil, fmt.Errorf("load credentials: %w", err)
	}

	return &app{store: store, accounts: mgr}, nil
}

// CredentialsPath returns the expected path for credentials.json.
func CredentialsPath() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.json"), nil
}

func (a *app) close() {
	a.store.Close() //nolint:errcheck
}

// gmailClientForAccount returns a gmail.Client for the given account ID.
func (a *app) gmailClientForAccount(ctx context.Context, accountID string) (*gmail.Client, error) {
	httpClient, err := a.accounts.GetClient(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return gmail.NewClient(ctx, httpClient, accountID)
}

// inboxService returns an InboxService for an account.
func (a *app) inboxService(ctx context.Context, accountID string) (*mail.InboxService, error) {
	client, err := a.gmailClientForAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return mail.NewInboxService(a.store, client), nil
}

// syncManager returns a SyncManager for an account.
func (a *app) syncManager(ctx context.Context, accountID string) (*mailsync.Manager, error) {
	client, err := a.gmailClientForAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return mailsync.NewManager(a.store, client), nil
}

// contactsService returns a contacts.Service for an account.
func (a *app) contactsService(ctx context.Context, accountID string) (*contacts.Service, error) {
	httpClient, err := a.accounts.GetClient(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return contacts.NewService(ctx, a.store, accountID, httpClient)
}

// dataDir returns (and creates if needed) the application data directory.
func dataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".mailrat")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

func parseJSONBytes(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
