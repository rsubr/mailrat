package accounts

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

type oauthFlow struct {
	cfg         *oauth2.Config
	server      *http.Server
	listener    net.Listener
	redirectURL string
	authURL     string
	codeCh      chan string
	serverErrCh chan error
}

// runOAuthFlow runs the OAuth2 consent flow and returns a token.
//
// Two modes:
//   - Browser mode: starts a local redirect server and opens the browser.
//     When Google redirects back, the code is captured automatically.
//   - Manual mode (no browser): prints the auth URL and asks the user to
//     paste the full redirect URL from their browser's address bar.
//     Google will redirect to localhost (which fails), but the URL with
//     the ?code= parameter is visible and can be copy-pasted.
func runOAuthFlow(cfg *oauth2.Config) (*oauth2.Token, error) {
	flow, err := newOAuthFlow(cfg)
	if err != nil {
		return nil, err
	}
	defer flow.shutdown()

	if err := flow.startCallbackServer(); err != nil {
		return nil, err
	}

	if flow.tryBrowserFlow() {
		token, err := flow.waitForBrowserAuthentication()
		switch {
		case err == nil:
			return token, nil
		case err != errSwitchToManual:
			return nil, err
		}
	}

	return flow.manualAuthentication()
}

func newOAuthFlow(cfg *oauth2.Config) (*oauthFlow, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start local listener: %w", err)
	}

	redirectURL := fmt.Sprintf("http://127.0.0.1:%d/callback", listener.Addr().(*net.TCPAddr).Port)
	cfg.RedirectURL = redirectURL

	return &oauthFlow{
		cfg:         cfg,
		listener:    listener,
		redirectURL: redirectURL,
		authURL:     cfg.AuthCodeURL(oauthStateToken, oauth2.AccessTypeOffline, oauth2.ApprovalForce),
		codeCh:      make(chan string, 1),
		serverErrCh: make(chan error, 1),
	}, nil
}

func (f *oauthFlow) startCallbackServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", f.handleCallback)
	f.server = &http.Server{Handler: mux}

	go func() {
		if err := f.server.Serve(f.listener); err != nil && err != http.ErrServerClosed {
			f.serverErrCh <- err
		}
	}()
	return nil
}

func (f *oauthFlow) shutdown() {
	if f.server == nil {
		return
	}
	_ = f.server.Shutdown(context.Background())
}

func (f *oauthFlow) handleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code parameter", http.StatusBadRequest)
		f.serverErrCh <- fmt.Errorf("callback received no code")
		return
	}
	fmt.Fprintln(w, callbackSuccessPage)
	f.codeCh <- code
}

func (f *oauthFlow) tryBrowserFlow() bool {
	return tryOpenBrowser(f.authURL)
}

var errSwitchToManual = fmt.Errorf("switch to manual auth")

func (f *oauthFlow) waitForBrowserAuthentication() (*oauth2.Token, error) {
	fmt.Println()
	fmt.Println("  A browser window has been opened for authentication.")
	fmt.Printf("  Waiting for Google to redirect to localhost:%s ...\n", strings.TrimPrefix(f.redirectURL, "http://127.0.0.1:"))
	fmt.Println()
	fmt.Println("  If the browser did not open, or you are on a remote machine,")
	fmt.Println("  press Enter to switch to manual URL entry instead.")
	fmt.Println()

	manualCh := make(chan struct{}, 1)
	go func() {
		_, _ = bufio.NewReader(os.Stdin).ReadBytes('\n')
		manualCh <- struct{}{}
	}()

	select {
	case code := <-f.codeCh:
		return exchangeCode(f.cfg, code)
	case err := <-f.serverErrCh:
		return nil, err
	case <-manualCh:
		return nil, errSwitchToManual
	case <-time.After(browserAuthTimeout * time.Minute):
		return nil, fmt.Errorf("timed out waiting for browser authentication")
	}
}

func (f *oauthFlow) manualAuthentication() (*oauth2.Token, error) {
	printManualAuthInstructions(f.authURL)

	stdinCh := make(chan string, 1)
	stdinErrCh := make(chan error, 1)
	go readManualRedirectURL(stdinCh, stdinErrCh)

	select {
	case code := <-f.codeCh:
		fmt.Println("  ✓ Received via browser callback.")
		return exchangeCode(f.cfg, code)
	case err := <-f.serverErrCh:
		return nil, err
	case raw := <-stdinCh:
		code, err := parseCodeFromRedirectURL(raw)
		if err != nil {
			return nil, fmt.Errorf("could not extract code from pasted URL: %w\n\n  Make sure you pasted the full URL from the browser address bar.", err)
		}
		return exchangeCode(f.cfg, code)
	case err := <-stdinErrCh:
		return nil, err
	case <-time.After(manualAuthTimeout * time.Minute):
		return nil, fmt.Errorf("timed out waiting for manual authentication")
	}
}

func readManualRedirectURL(stdinCh chan<- string, errCh chan<- error) {
	fmt.Print("  Paste redirect URL: ")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		stdinCh <- strings.TrimSpace(scanner.Text())
		return
	}
	if err := scanner.Err(); err != nil {
		errCh <- err
	}
}

func printManualAuthInstructions(authURL string) {
	fmt.Println()
	fmt.Println("  ┌─ Manual authentication ─────────────────────────────────────────┐")
	fmt.Println("  │                                                                   │")
	fmt.Println("  │  1. Open this URL in any browser (on any device):                │")
	fmt.Println("  │                                                                   │")
	fmt.Printf("  │  %s\n", authURL)
	fmt.Println("  │                                                                   │")
	fmt.Println("  │  2. Sign in and grant access.                                    │")
	fmt.Println("  │                                                                   │")
	fmt.Println("  │  3. The browser will show a 'connection refused' error or a      │")
	fmt.Println("  │     blank page. That is expected.                                │")
	fmt.Println("  │                                                                   │")
	fmt.Println("  │  4. Copy the FULL URL from your browser's address bar and        │")
	fmt.Println("  │     paste it below.                                              │")
	fmt.Println("  │                                                                   │")
	fmt.Println("  └───────────────────────────────────────────────────────────────────┘")
	fmt.Println()
}

// parseCodeFromRedirectURL extracts the ?code= parameter from a redirect URL.
// Accepts both a full URL and a bare code string.
func parseCodeFromRedirectURL(raw string) (string, error) {
	if strings.HasPrefix(raw, "http") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("invalid URL: %w", err)
		}
		code := u.Query().Get("code")
		if code == "" {
			return "", fmt.Errorf("no 'code' parameter in URL")
		}
		return code, nil
	}
	if raw != "" {
		return raw, nil
	}
	return "", fmt.Errorf("empty input")
}

func oauthContext() context.Context {
	return context.Background()
}
