package desktop

import (
	"testing"

	"github.com/mailrat/mailrat/internal/storage"
)

func TestSnippetsAreUnescapedOnce(t *testing.T) {
	// Gmail HTML-escapes snippets; "&amp;amp;" is a literal "&amp;" in the mail.
	const raw = "tomorrow&#39;s meeting &amp; Thanks &amp;amp; Q&lt;A"
	const want = "tomorrow's meeting & Thanks &amp; Q<A"
	if got := threadSummaryToItem(storage.ThreadSummary{Snippet: raw}).Snippet; got != want {
		t.Errorf("thread snippet = %q, want %q", got, want)
	}
	if got := messageToItem(storage.Message{Snippet: raw}).Snippet; got != want {
		t.Errorf("message snippet = %q, want %q", got, want)
	}
}

func TestStoredMojibakeBodyIsRepaired(t *testing.T) {
	got := messageToItem(storage.Message{Body: "<p>Coimbatore â€“ 641</p>"}).Body
	if want := "<p>Coimbatore – 641</p>"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}
