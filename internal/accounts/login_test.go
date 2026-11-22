package accounts

import (
	"net/http"
	"strings"
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
	return &Login{flow: flow}
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
	if err := l.Submit("http://127.0.0.1:1/callback?state=x&code=4%2Fxyz"); err != nil {
		t.Fatal(err)
	}
	if c := <-l.flow.codeCh; c != "4/xyz" {
		t.Fatalf("code = %q", c)
	}
	if err := l.Submit("http://127.0.0.1:1/callback?nocode=1"); err == nil {
		t.Fatal("expected error for URL without code")
	}
}
