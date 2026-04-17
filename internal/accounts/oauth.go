package accounts

import (
	"encoding/json"
	"fmt"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	googleapi "google.golang.org/api/gmail/v1"
	peopleapi "google.golang.org/api/people/v1"
)

const (
	oauthStateToken     = "state-token"
	browserAuthTimeout  = 3
	manualAuthTimeout   = 10
	callbackSuccessPage = `<html><body style="font-family:sans-serif;padding:2em"><h2>✓ Authentication successful</h2><p>You can close this tab and return to the terminal.</p></body></html>`
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

func exchangeCode(cfg *oauth2.Config, code string) (*oauth2.Token, error) {
	token, err := cfg.Exchange(oauthContext(), code)
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
