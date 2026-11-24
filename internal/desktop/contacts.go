package desktop

import (
	"fmt"
	"net/mail"
	"sort"
	"strings"
	"unicode"

	"github.com/mailrat/mailrat/internal/contacts"
	"github.com/mailrat/mailrat/internal/storage"
)

// searchPool is how many substring matches are fetched before ranking, so a
// prefix match is not cut off by the SQL limit.
const searchPool = 500

// rankContacts orders matches for autocomplete and keeps at most limit: names
// starting with the query first, then any name word or the address starting with
// it, then plain substring matches. Ties keep their (name) order.
func rankContacts(items []ContactItem, query string, limit int) []ContactItem {
	q := strings.ToLower(strings.TrimSpace(query))
	tier := func(c ContactItem) int {
		name, email := strings.ToLower(c.Name), strings.ToLower(c.Email)
		if strings.HasPrefix(name, q) {
			return 0
		}
		if strings.HasPrefix(email, q) {
			return 1
		}
		for _, w := range strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			if strings.HasPrefix(w, q) {
				return 1
			}
		}
		return 2
	}
	out := append([]ContactItem(nil), items...)
	sort.SliceStable(out, func(i, j int) bool { return tier(out[i]) < tier(out[j]) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// ListContacts returns every contact of an account ("unified" or "" for all).
func (a *App) ListContacts(accountID string) ([]ContactItem, error) {
	if a.store == nil {
		return nil, nil
	}
	if accountID == "" || accountID == "unified" {
		cs, err := a.store.SearchAllContacts("", 1<<30)
		return contactsToItems(cs), err
	}
	cs, err := a.store.ListContacts(accountID)
	return contactsToItems(cs), err
}

func (a *App) contactsSvc(accountID string) (*contacts.Service, error) {
	httpClient, err := a.accountMgr().GetClient(a.ctx, accountID)
	if err != nil {
		return nil, err
	}
	return contacts.NewService(a.ctx, a.store, accountID, httpClient)
}

// SaveContact creates a contact (empty ID) or updates one, in Google and locally.
func (a *App) SaveContact(c ContactItem) (ContactItem, error) {
	if strings.TrimSpace(c.Email) == "" {
		return ContactItem{}, fmt.Errorf("email is required")
	}
	if ad, err := mail.ParseAddress(strings.TrimSpace(c.Email)); err != nil || ad.Address != strings.TrimSpace(c.Email) {
		return ContactItem{}, fmt.Errorf("%q is not a valid email address", strings.TrimSpace(c.Email))
	}
	svc, err := a.contactsSvc(c.AccountID)
	if err != nil {
		return ContactItem{}, err
	}
	if c.ID == "" {
		sc, err := svc.Create(a.ctx, c.Name, c.Email, c.Phone)
		return contactsToItems([]storage.Contact{sc})[0], err
	}
	sc, err := svc.Update(a.ctx, c.ID, c.Name, c.Email, c.Phone)
	return contactsToItems([]storage.Contact{sc})[0], err
}

// DeleteContact removes a contact from Google and locally.
func (a *App) DeleteContact(accountID, id string) error {
	svc, err := a.contactsSvc(accountID)
	if err != nil {
		return err
	}
	return svc.Delete(a.ctx, id)
}

// FrequentAddresses returns the addresses seen most in recent mail (sent or
// received), most frequent first, with Count set. Names come from the headers.
func (a *App) FrequentAddresses(accountID string, limit int) ([]ContactItem, error) {
	if a.store == nil {
		return nil, nil
	}
	headers, err := a.store.RecentAddressHeaders(accountID, 3000)
	if err != nil {
		return nil, err
	}
	return countAddresses(headers, limit), nil
}

// countAddresses tallies the addresses in raw From/To headers.
func countAddresses(headers []string, limit int) []ContactItem {
	seen := map[string]*ContactItem{}
	for _, h := range headers {
		list, err := mail.ParseAddressList(h)
		if err != nil {
			continue
		}
		for _, ad := range list {
			k := strings.ToLower(ad.Address)
			if it := seen[k]; it != nil {
				it.Count++
				if it.Name == "" {
					it.Name = ad.Name
				}
				continue
			}
			seen[k] = &ContactItem{Name: ad.Name, Email: ad.Address, Count: 1}
		}
	}
	out := make([]ContactItem, 0, len(seen))
	for _, it := range seen {
		out = append(out, *it)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Email < out[j].Email
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
