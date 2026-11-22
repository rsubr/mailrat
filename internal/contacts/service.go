package contacts

import (
	"context"
	"fmt"
	"net/http"

	"google.golang.org/api/option"
	peoplev1 "google.golang.org/api/people/v1"

	"github.com/mailrat/mailrat/internal/storage"
)

// Service syncs and manages contacts via the Google People API.
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

// Sync fetches all contacts from the People API and stores them locally.
func (s *Service) Sync(ctx context.Context) error {
	pageToken := ""
	synced := 0

	for {
		call := s.svc.People.Connections.List("people/me").
			PersonFields("names,emailAddresses,phoneNumbers").
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
			etag := person.Etag
			phone := ""
			if len(person.PhoneNumbers) > 0 {
				phone = person.PhoneNumbers[0].Value
			}
			for _, email := range person.EmailAddresses {
				if email.Value == "" {
					continue
				}
				c := storage.Contact{
					ID:           person.ResourceName + ":" + email.Value,
					AccountID:    s.accountID,
					Name:         name,
					Email:        email.Value,
					ResourceName: person.ResourceName,
					Etag:         etag,
					Phone:        phone,
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

// Create creates a new contact in Google and saves it locally.
func (s *Service) Create(ctx context.Context, name, email, phone string) (storage.Contact, error) {
	person := &peoplev1.Person{
		Names: []*peoplev1.Name{
			{GivenName: name},
		},
		EmailAddresses: []*peoplev1.EmailAddress{
			{Value: email},
		},
		PhoneNumbers: phoneNumbers(phone),
	}

	created, err := s.svc.People.CreateContact(person).
		PersonFields("names,emailAddresses,phoneNumbers").
		Context(ctx).Do()
	if err != nil {
		return storage.Contact{}, fmt.Errorf("create contact: %w", err)
	}

	c := storage.Contact{
		ID:           created.ResourceName + ":" + email,
		AccountID:    s.accountID,
		Name:         name,
		Email:        email,
		Phone:        phone,
		ResourceName: created.ResourceName,
		Etag:         created.Etag,
	}
	if err := s.store.UpsertContact(c); err != nil {
		return storage.Contact{}, err
	}
	return c, nil
}

// Update updates the name and/or email of an existing contact.
// id is the local contact ID (resource_name:email).
func (s *Service) Update(ctx context.Context, id, newName, newEmail, newPhone string) (storage.Contact, error) {
	existing, err := s.store.GetContact(s.accountID, id)
	if err != nil {
		return storage.Contact{}, fmt.Errorf("contact %q not found locally; run: mailrat contacts list --sync", id)
	}

	person := &peoplev1.Person{
		ResourceName: existing.ResourceName,
		Etag:         existing.Etag,
		Names: []*peoplev1.Name{
			{GivenName: newName},
		},
		EmailAddresses: []*peoplev1.EmailAddress{
			{Value: newEmail},
		},
		PhoneNumbers: phoneNumbers(newPhone),
	}

	updated, err := s.svc.People.UpdateContact(existing.ResourceName, person).
		UpdatePersonFields("names,emailAddresses,phoneNumbers").
		Context(ctx).Do()
	if err != nil {
		return storage.Contact{}, fmt.Errorf("update contact: %w", err)
	}

	// ID changes if the email changed.
	newID := updated.ResourceName + ":" + newEmail
	if newID != id {
		_ = s.store.DeleteContact(s.accountID, id)
	}

	c := storage.Contact{
		ID:           newID,
		AccountID:    s.accountID,
		Name:         newName,
		Email:        newEmail,
		Phone:        newPhone,
		ResourceName: updated.ResourceName,
		Etag:         updated.Etag,
	}
	if err := s.store.UpsertContact(c); err != nil {
		return storage.Contact{}, err
	}
	return c, nil
}

// Delete deletes a contact from Google and removes it locally.
// id is the local contact ID (resource_name:email).
func (s *Service) Delete(ctx context.Context, id string) error {
	existing, err := s.store.GetContact(s.accountID, id)
	if err != nil {
		return fmt.Errorf("contact %q not found locally; run: mailrat contacts list --sync", id)
	}

	if _, err := s.svc.People.DeleteContact(existing.ResourceName).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete contact: %w", err)
	}

	return s.store.DeleteContact(s.accountID, id)
}

// phoneNumbers is the People API phone list for a single number; empty clears it.
func phoneNumbers(p string) []*peoplev1.PhoneNumber {
	if p == "" {
		return nil
	}
	return []*peoplev1.PhoneNumber{{Value: p}}
}
