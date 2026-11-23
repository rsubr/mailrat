package gmail

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGetHistoryListDeletedIDs(t *testing.T) {
	body := `{"historyId":"9","history":[
		{"messagesAdded":[{"message":{"id":"new1"}}]},
		{"messagesDeleted":[{"message":{"id":"new1"}},{"message":{"id":"old1"}}]}]}`
	hc := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}},
			Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	c, err := NewClient(context.Background(), hc, "a1")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.GetHistoryList(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.AddedIDs) != 0 {
		t.Fatalf("add-then-delete still fetched: %v", resp.AddedIDs)
	}
	if !slices.Equal(resp.DeletedIDs, []string{"old1"}) {
		t.Fatalf("DeletedIDs = %v", resp.DeletedIDs)
	}
}
