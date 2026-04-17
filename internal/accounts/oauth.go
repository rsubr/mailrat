package accounts

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	googleapi "google.golang.org/api/gmail/v1"
	peopleapi "google.golang.org/api/people/v1"
)

var scopes = []string{
	googleapi.GmailReadonlyScope,
	googleapi.GmailModifyScope,
	googleapi.GmailSendScope,
	peopleapi.ContactsReadonlyScope,
}

// oauthConfig builds an OAuth2 config from credentials JSON.
// credentials is the content of a Google OAuth2 credentials.json file.
func oauthConfig(credentials []byte) (*oauth2.Config, error) {
	cfg, err := google.ConfigFromJSON(credentials, scopes...)
	if err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	return cfg, nil
}

// runOAuthFlow launches the browser consent flow and returns a token.
func runOAuthFlow(cfg *oauth2.Config) (*oauth2.Token, error) {
	// Start local server on a random port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	cfg.RedirectURL = redirectURL

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	srv := &http.Server{}
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback")
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		fmt.Fprintln(w, "<html><body><h2>Authentication successful! You can close this tab.</h2></body></html>")
		codeCh <- code
	})
	srv.Handler = mux

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	defer srv.Shutdown(context.Background()) //nolint:errcheck

	authURL := cfg.AuthCodeURL("state", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	fmt.Fprintf(os.Stderr, "Opening browser for authentication...\nIf it does not open, visit:\n%s\n\n", authURL)
	openBrowser(authURL)

	select {
	case code := <-codeCh:
		token, err := cfg.Exchange(context.Background(), code)
		if err != nil {
			return nil, fmt.Errorf("token exchange: %w", err)
		}
		return token, nil
	case err := <-errCh:
		return nil, err
	}
}

func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd, args = "open", []string{url}
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		cmd, args = "xdg-open", []string{url}
	}
	exec.Command(cmd, args...).Start() //nolint:errcheck
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
