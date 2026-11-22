package ui

import (
	"testing"
	"time"
)

func TestHTMLToText(t *testing.T) {
	got := htmlToText(`<html><head><style>p{}</style></head><body><div>Hello <b>Ben</b>,</div><p>12PM &amp; more</p><blockquote>old<br>text</blockquote><ul><li>a</li><li>b</li></ul></body></html>`)
	want := "Hello Ben,\n\n12PM & more\n\n> old\n> text\n\n• a\n• b"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if htmlToText("  plain\ntext ") != "plain\ntext" {
		t.Fatal("plain text should pass through")
	}
}

func TestFormatDates(t *testing.T) {
	now := time.Date(2026, 11, 22, 15, 0, 0, 0, time.UTC)
	ms := func(d time.Time) int64 { return d.UnixMilli() }
	cases := []struct {
		t    time.Time
		want string
	}{
		{time.Date(2026, 11, 22, 10, 58, 0, 0, time.UTC), "10:58 AM"},
		{time.Date(2026, 11, 21, 23, 0, 0, 0, time.UTC), "Yesterday"},
		{time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC), "Wednesday"},
		{time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC), "Mar 2"},
		{time.Date(2024, 3, 2, 9, 0, 0, 0, time.UTC), "3/2/24"},
	}
	for _, c := range cases {
		if got := formatListDate(ms(c.t), now); got != c.want {
			t.Errorf("%v: got %q want %q", c.t, got, c.want)
		}
	}
	if got := formatMessageDate(ms(time.Date(2026, 11, 22, 10, 58, 0, 0, time.UTC)), now); got != "Today at 10:58AM" {
		t.Errorf("got %q", got)
	}
}

func TestNamesAndInitials(t *testing.T) {
	if displayName(`"Ben Johnson" <ben@x.com>`) != "Ben Johnson" || displayName("<a@b.c>") != "a@b.c" {
		t.Fatal("displayName")
	}
	if initials("Ben Johnson") != "BJ" || initials("") != "?" {
		t.Fatal("initials")
	}
}
