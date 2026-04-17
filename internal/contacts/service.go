package contacts

import (
	"context"
	"fmt"
	"net/http"

	"google.golang.org/api/option"
	peoplev1 "google.golang.org/api/people/v1"

	"github.com/mailrat/mailrat/internal/storage"
)

// Service syncs and searches contacts via the Google People API.
type Service struct {
	store     *storage.Store
	accountID string
	svc       *peoplev1.Service
}

// NewService creates a contacts Service for one account.
func NewService(ctx context.Context, store *storage.Store, accountID string, httpClient *http.Client) (*Service, error) {
	svc, err := peoplev1.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("create people service: %w", err)
	}
	return &Service{store: store, accountID: accountID, svc: svc}, nil
}

// Sync fetches the user's contacts from the People API and stores them locally.
func (s *Service) Sync(ctx context.Context) error {
	pageToken := ""
	synced := 0

	for {
		call := s.svc.People.Connections.List("people/me").
			PersonFields("names,emailAddresses").
			PageSize(1000)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("list connections: %w", err)
		}

		for _, person := range resp.Connections {
			name := ""
			if len(person.Names) > 0 {
				name = person.Names[0].DisplayName
			}
			for _, email := range person.EmailAddresses {
				if email.Value == "" {
					continue
				}
				c := storage.Contact{
					ID:        person.ResourceName + ":" + email.Value,
					AccountID: s.accountID,
					Name:      name,
					Email:     email.Value,
				}
				if err := s.store.UpsertContact(c); err != nil {
					return err
				}
				synced++
			}
		}

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	fmt.Printf("Contacts sync complete: %d contacts stored\n", synced)
	return nil
}

// Search returns contacts matching the query string (name or email).
func (s *Service) Search(query string, limit int) ([]storage.Contact, error) {
	return s.store.SearchContacts(s.accountID, query, limit)
}

// List returns all contacts for the account.
func (s *Service) List() ([]storage.Contact, error) {
	return s.store.ListContacts(s.accountID)
}
