package accounts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
)

type oauthFlow struct {
	cfg         *oauth2.Config
	server      *http.Server
	listener    net.Listener
	redirectURL string
	authURL     string
	state       string
	codeCh      chan string
	serverErrCh chan error
}

func newOAuthFlow(cfg *oauth2.Config) (*oauthFlow, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start local listener: %w", err)
	}

	redirectURL := fmt.Sprintf("http://127.0.0.1:%d/callback", listener.Addr().(*net.TCPAddr).Port)
	cfg.RedirectURL = redirectURL

	state, err := randomState()
	if err != nil {
		listener.Close()
		return nil, err
	}

	return &oauthFlow{
		state:       state,
		cfg:         cfg,
		listener:    listener,
		redirectURL: redirectURL,
		authURL:     cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce),
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
	q := r.URL.Query()
	if q.Get("state") != f.state {
		// Not our flow (stale tab, or another local process probing the port).
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	code := q.Get("code")
	if code == "" {
		http.Error(w, "missing code parameter", http.StatusBadRequest)
		reason := q.Get("error")
		if reason == "" {
			reason = "callback received no code"
		}
		select {
		case f.serverErrCh <- fmt.Errorf("sign-in failed: %s", reason):
		default:
		}
		return
	}
	fmt.Fprintln(w, callbackSuccessPage)
	select {
	case f.codeCh <- code:
	default:
	}
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

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}
	return hex.EncodeToString(b), nil
}
