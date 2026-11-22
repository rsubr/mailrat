package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/mailrat/mailrat/internal/desktop"
)

func TestBuildContactRows(t *testing.T) {
	book := []desktop.ContactItem{{ID: "1", Name: "Ann", Email: "ann@x.com", Phone: "555"}, {ID: "2", Name: "Bob", Email: "bob@x.com"}}
	freq := []desktop.ContactItem{{Email: "BOB@x.com", Count: 9}, {Email: "zed@x.com", Count: 3}}
	rows := buildContactRows(book, freq, "")
	var got []string
	for _, r := range rows {
		if r.Header != "" {
			got = append(got, "#"+r.Header)
		} else {
			got = append(got, r.C.Email)
		}
	}
	want := []string{"#Frequently contacted", "bob@x.com", "#Contacts (2)", "ann@x.com", "bob@x.com", "#Other contacts", "zed@x.com"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	if r := buildContactRows(book, freq, "555"); len(r) != 1 || r[0].C.ID != "1" {
		t.Fatalf("phone search: %+v", r)
	}
}

func TestContactsWindowOpens(t *testing.T) {
	a := test.NewTempApp(t)
	fb := &fakeBackend{book: []desktop.ContactItem{{ID: "1", AccountID: "a1", Name: "Ann", Email: "ann@x.com"}}}
	u := &UI{app: a, inline: true, be: fb, activeAccount: unified, accounts: []desktop.AccountItem{{ID: "a1", Email: "n@p.com"}}}
	u.openContacts()
	if u.contactsWin == nil {
		t.Fatal("window not opened")
	}
	u.contactsWin.Close()
}
