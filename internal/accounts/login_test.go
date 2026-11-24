package accounts

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func testFlow(t *testing.T) *Login {
	t.Helper()
	cfg := &oauth2.Config{
		ClientID: "id", Scopes: []string{"s"},
		Endpoint: oauth2.Endpoint{AuthURL: "https://example.test/auth", TokenURL: "https://example.test/token"},
	}
	flow, err := newOAuthFlow(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := flow.startCallbackServer(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(flow.shutdown)
	return &Login{flow: flow, done: make(chan struct{})}
}

func TestCallbackChecksState(t *testing.T) {
	l := testFlow(t)
	if !strings.Contains(l.AuthURL(), "state="+l.flow.state) || l.flow.state == "" {
		t.Fatalf("auth URL missing state: %s", l.AuthURL())
	}

	bad, err := http.Get(l.flow.redirectURL + "?code=evil&state=wrong")
	if err != nil {
		t.Fatal(err)
	}
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong state: status %d", bad.StatusCode)
	}
	select {
	case c := <-l.flow.codeCh:
		t.Fatalf("code %q accepted despite bad state", c)
	default:
	}

	ok, err := http.Get(l.flow.redirectURL + "?code=abc&state=" + l.flow.state)
	if err != nil {
		t.Fatal(err)
	}
	ok.Body.Close()
	select {
	case c := <-l.flow.codeCh:
		if c != "abc" {
			t.Fatalf("code = %q", c)
		}
	case <-time.After(time.Second):
		t.Fatal("code not delivered")
	}
}

func TestCallbackReportsDeniedConsent(t *testing.T) {
	l := testFlow(t)
	resp, err := http.Get(l.flow.redirectURL + "?error=access_denied&state=" + l.flow.state)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	select {
	case err := <-l.flow.serverErrCh:
		if !strings.Contains(err.Error(), "access_denied") {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("no error reported")
	}
}

func TestSubmitPastedRedirect(t *testing.T) {
	l := testFlow(t)
	if err := l.Submit("http://127.0.0.1:1/callback?state=" + l.flow.state + "&code=4%2Fxyz"); err != nil {
		t.Fatal(err)
	}
	if c := <-l.flow.codeCh; c != "4/xyz" {
		t.Fatalf("code = %q", c)
	}
	if err := l.Submit("http://127.0.0.1:1/callback?state=" + l.flow.state); err == nil {
		t.Fatal("expected error for URL without code")
	}
}

func TestSubmitTrimsWhitespace(t *testing.T) {
	l := testFlow(t)
	if err := l.Submit("  \n http://127.0.0.1:1/callback?code=abc&state=" + l.flow.state + " \r\n"); err != nil {
		t.Fatal(err)
	}
	if c := <-l.flow.codeCh; c != "abc" {
		t.Fatalf("code = %q", c)
	}
	if err := l.Submit("  4/bare \n"); err != nil {
		t.Fatal(err)
	}
	if c := <-l.flow.codeCh; c != "4/bare" {
		t.Fatalf("bare code = %q", c)
	}
	if err := l.Submit(" \n "); err == nil {
		t.Fatal("blank paste accepted")
	}
}

func TestSubmitRejectsForeignState(t *testing.T) {
	l := testFlow(t)
	for _, raw := range []string{
		"http://127.0.0.1:1/callback?code=evil&state=wrong",
		"http://127.0.0.1:1/callback?code=evil",
	} {
		if err := l.Submit(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	select {
	case c := <-l.flow.codeCh:
		t.Fatalf("code %q queued despite bad state", c)
	default:
	}
}

func TestSubmitReportsGoogleError(t *testing.T) {
	l := testFlow(t)
	err := l.Submit("http://127.0.0.1:1/callback?error=access_denied&state=" + l.flow.state)
	if err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("err = %v", err)
	}
}

func TestSubmitWhileCodePending(t *testing.T) {
	l := testFlow(t)
	if err := l.Submit("4/one"); err != nil {
		t.Fatal(err)
	}
	if err := l.Submit("4/two"); err == nil {
		t.Fatal("second submit silently dropped")
	}
}

// A code Google rejects must not tear down the sign-in: the listener stays up
// and the browser's real redirect can still be delivered.
func TestFailedExchangeKeepsListener(t *testing.T) {
	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
	}))
	defer ts.Close()
	l := testFlow(t)
	l.flow.cfg.Endpoint.TokenURL = ts.URL
	failed := make(chan error, 4)
	l.OnError = func(err error) { failed <- err }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waitErr := make(chan error, 1)
	go func() { _, err := l.Wait(ctx); waitErr <- err }()

	if err := l.Submit("4/stale"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-failed:
		if !strings.Contains(err.Error(), "token exchange failed") {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("exchange error not reported")
	}
	select {
	case err := <-waitErr:
		t.Fatalf("Wait returned after a failed exchange: %v", err)
	default:
	}
	// The real redirect still reaches the live listener and is exchanged.
	resp, err := http.Get(l.flow.redirectURL + "?code=fresh&state=" + l.flow.state)
	if err != nil {
		t.Fatalf("listener closed after failed exchange: %v", err)
	}
	resp.Body.Close()
	select {
	case <-failed:
	case <-time.After(3 * time.Second):
		t.Fatal("second code not exchanged")
	}
	if hits.Load() < 2 {
		t.Fatalf("token endpoint hits = %d", hits.Load())
	}
	cancel()
	select {
	case <-waitErr:
	case <-time.After(3 * time.Second):
		t.Fatal("Wait did not stop on cancel")
	}
	select {
	case <-l.Done():
	case <-time.After(time.Second):
		t.Fatal("Done not closed")
	}
}

func TestValidateCredentialsRequiresInstalledClient(t *testing.T) {
	body := `{"%s":{"client_id":"i","client_secret":"s","auth_uri":"https://a/auth","token_uri":"https://a/token","redirect_uris":["http://localhost"]}}`
	if err := ValidateCredentials([]byte(fmt.Sprintf(body, "installed"))); err != nil {
		t.Fatalf("installed rejected: %v", err)
	}
	if err := ValidateCredentials([]byte(fmt.Sprintf(body, "web"))); err == nil {
		t.Fatal("web client accepted; its redirect URIs cannot match the loopback port")
	}
}
