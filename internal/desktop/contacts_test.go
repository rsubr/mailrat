package desktop

import "testing"

func emails(items []ContactItem) []string {
	var out []string
	for _, c := range items {
		out = append(out, c.Email)
	}
	return out
}

func TestRankContactsPrefixFirst(t *testing.T) {
	in := []ContactItem{ // name order, as SQL returns them
		{Name: "Kamaraj", Email: "k@x.com"},
		{Name: "Raja", Email: "r@x.com"},
		{Name: "Rajaram", Email: "rr@x.com"},
		{Name: "Dr Raj Kumar", Email: "d@x.com"},
		{Email: "raj.s@x.com"},
	}
	got := emails(rankContacts(in, "raj", 10))
	want := []string{"r@x.com", "rr@x.com", "d@x.com", "raj.s@x.com", "k@x.com"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	if n := len(rankContacts(in, "raj", 2)); n != 2 {
		t.Fatalf("limit: %d", n)
	}
}

func TestCountAddresses(t *testing.T) {
	got := countAddresses([]string{`Ann <a@x.com>, b@x.com`, `ANN <A@x.com>`, `junk`}, 10)
	if len(got) != 2 || got[0].Email != "a@x.com" || got[0].Count != 2 || got[0].Name != "Ann" {
		t.Fatalf("%+v", got)
	}
}
