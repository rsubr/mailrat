package accounts

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	googleapi "google.golang.org/api/gmail/v1"
	peopleapi "google.golang.org/api/people/v1"
)

var scopes = []string{
	googleapi.GmailReadonlyScope,
	googleapi.GmailModifyScope,
	googleapi.GmailSendScope,
	peopleapi.ContactsScope, // read + write (create/update/delete)
}

// oauthConfig builds an OAuth2 config from credentials JSON.
func oauthConfig(credentials []byte) (*oauth2.Config, error) {
	cfg, err := google.ConfigFromJSON(credentials, scopes...)
	if err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	return cfg, nil
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
	// Start local redirect server on a random port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start local listener: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	cfg.RedirectURL = redirectURL

	authURL := cfg.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	codeCh := make(chan string, 1)
	serverErrCh := make(chan error, 1)

	// Start the local callback server.
	srv := &http.Server{}
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code parameter", http.StatusBadRequest)
			serverErrCh <- fmt.Errorf("callback received no code")
			return
		}
		fmt.Fprintln(w, `<html><body style="font-family:sans-serif;padding:2em">
			<h2>✓ Authentication successful</h2>
			<p>You can close this tab and return to the terminal.</p>
		</body></html>`)
		codeCh <- code
	})
	srv.Handler = mux
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			serverErrCh <- err
		}
	}()
	defer srv.Shutdown(context.Background()) //nolint:errcheck

	// Try to open a browser. If it fails or is unavailable, fall back to manual.
	browserOpened := tryOpenBrowser(authURL)

	if browserOpened {
		fmt.Println()
		fmt.Println("  A browser window has been opened for authentication.")
		fmt.Printf("  Waiting for Google to redirect to localhost:%d ...\n", port)
		fmt.Println()
		fmt.Println("  If the browser did not open, or you are on a remote machine,")
		fmt.Println("  press Enter to switch to manual URL entry instead.")
		fmt.Println()

		// Race: browser callback vs. user pressing Enter for manual mode.
		manualCh := make(chan struct{}, 1)
		go func() {
			bufio.NewReader(os.Stdin).ReadBytes('\n') //nolint:errcheck
			manualCh <- struct{}{}
		}()

		select {
		case code := <-codeCh:
			return exchangeCode(cfg, code)
		case <-manualCh:
			// Fall through to manual mode below.
		case <-time.After(3 * time.Minute):
			return nil, fmt.Errorf("timed out waiting for browser authentication")
		}
	}

	// Manual mode: print the URL and read the redirect URL from stdin.
	return manualFlow(cfg, authURL, codeCh)
}

// manualFlow handles the no-browser case: prints the auth URL and reads
// the redirect URL (containing ?code=) from stdin.
func manualFlow(cfg *oauth2.Config, authURL string, codeCh <-chan string) (*oauth2.Token, error) {
	fmt.Println()
	fmt.Println("  ┌─ Manual authentication ─────────────────────────────────────────┐")
	fmt.Println("  │                                                                   │")
	fmt.Println("  │  1. Open this URL in any browser (on any device):                │")
	fmt.Println("  │                                                                   │")
	fmt.Printf( "  │  %s\n", authURL)
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

	// Also accept an automatic callback if it somehow arrives.
	stdinCh := make(chan string, 1)
	stdinErrCh := make(chan error, 1)
	go func() {
		fmt.Print("  Paste redirect URL: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			stdinCh <- strings.TrimSpace(scanner.Text())
		} else if err := scanner.Err(); err != nil {
			stdinErrCh <- err
		}
	}()

	select {
	case code := <-codeCh:
		// Browser callback arrived after all (e.g. user switched tabs).
		fmt.Println("  ✓ Received via browser callback.")
		return exchangeCode(cfg, code)
	case raw := <-stdinCh:
		code, err := parseCodeFromRedirectURL(raw)
		if err != nil {
			return nil, fmt.Errorf("could not extract code from pasted URL: %w\n\n  Make sure you pasted the full URL from the browser address bar.", err)
		}
		return exchangeCode(cfg, code)
	case err := <-stdinErrCh:
		return nil, err
	case <-time.After(10 * time.Minute):
		return nil, fmt.Errorf("timed out waiting for manual authentication")
	}
}

// parseCodeFromRedirectURL extracts the ?code= parameter from a redirect URL.
// Accepts both a full URL and a bare code string.
func parseCodeFromRedirectURL(raw string) (string, error) {
	// If it looks like a URL, parse it.
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
	// Bare code string.
	if raw != "" {
		return raw, nil
	}
	return "", fmt.Errorf("empty input")
}

// tryOpenBrowser attempts to open url in the system browser.
// Returns true if the command launched without error.
func tryOpenBrowser(url string) bool {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd, args = "open", []string{url}
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		// Linux: try xdg-open, then sensible-browser, then x-www-browser.
		for _, bin := range []string{"xdg-open", "sensible-browser", "x-www-browser"} {
			if _, err := exec.LookPath(bin); err == nil {
				cmd, args = bin, []string{url}
				break
			}
		}
		if cmd == "" {
			return false
		}
	}
	err := exec.Command(cmd, args...).Start()
	return err == nil
}

func exchangeCode(cfg *oauth2.Config, code string) (*oauth2.Token, error) {
	token, err := cfg.Exchange(context.Background(), code)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}
	return token, nil
}

func tokenToJSON(t *oauth2.Token) (string, error) {
	b, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func tokenFromJSON(s string) (*oauth2.Token, error) {
	var t oauth2.Token
	if err := json.Unmarshal([]byte(s), &t); err != nil {
		return nil, err
	}
	return &t, nil
}
