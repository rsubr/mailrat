package accounts

import (
	"context"
	"fmt"
	"time"

	"github.com/mailrat/mailrat/internal/storage"
)

// Login is one in-progress browser sign-in for GUI callers. Unlike
// AddAccount it never touches stdin or stdout: the caller shows AuthURL, the
// loopback listener catches Google's redirect, and a pasted redirect URL
// (Submit) is the fallback when the browser cannot reach localhost.
type Login struct {
	m    *Manager
	flow *oauthFlow
	done chan struct{}

	// OnError, if set, is told about a code that could not be exchanged.
	OnError func(error)
}

// BeginLogin starts the loopback listener and returns a Login ready to wait on.
func (m *Manager) BeginLogin() (*Login, error) {
	cfg, err := oauthConfig(m.credentials)
	if err != nil {
		return nil, err
	}
	flow, err := newOAuthFlow(cfg)
	if err != nil {
		return nil, err
	}
	if err := flow.startCallbackServer(); err != nil {
		flow.shutdown()
		return nil, err
	}
	return &Login{m: m, flow: flow, done: make(chan struct{})}, nil
}

// AuthURL is the Google consent URL to open in a browser.
func (l *Login) AuthURL() string { return l.flow.authURL }

// OpenBrowser asks the OS to open the consent page; false means the caller
// should show AuthURL for the user to open themselves.
func (l *Login) OpenBrowser() bool { return tryOpenBrowser(l.flow.authURL) }

// Submit accepts a pasted redirect URL (or bare code) as an alternative to the
// automatic callback. Wait then completes with it. A rejected paste leaves the
// sign-in running so the user can paste again.
func (l *Login) Submit(raw string) error {
	code, err := l.flow.parseCodeFromRedirectURL(raw)
	if err != nil {
		return fmt.Errorf("could not read a sign-in code from that text: %w", err)
	}
	select {
	case l.flow.codeCh <- code:
		return nil
	default:
		return fmt.Errorf("a sign-in code is already being processed")
	}
}

// Done is closed once Wait has returned and released the listener.
func (l *Login) Done() <-chan struct{} { return l.done }

// Wait blocks until the browser callback or a Submit delivers a code, then
// exchanges it and saves the account. A failed exchange (bad or expired code)
// is passed to OnError and the wait continues, so the listener stays open for
// another try. It always releases the listener on return.
func (l *Login) Wait(ctx context.Context) (storage.Account, error) {
	defer close(l.done)
	defer l.flow.shutdown()
	timeout := time.After(manualAuthTimeout * time.Minute)
	for {
		var code string
		select {
		case code = <-l.flow.codeCh:
		case err := <-l.flow.serverErrCh:
			return storage.Account{}, err
		case <-ctx.Done():
			return storage.Account{}, ctx.Err()
		case <-timeout:
			return storage.Account{}, fmt.Errorf("timed out waiting for sign-in")
		}
		token, err := exchangeCode(ctx, l.flow.cfg, code)
		if err == nil {
			return l.m.saveAccount(ctx, l.flow.cfg, token)
		}
		if ctx.Err() != nil {
			return storage.Account{}, ctx.Err()
		}
		if l.OnError != nil {
			l.OnError(err)
		}
	}
}
