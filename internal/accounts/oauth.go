package accounts

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	googleapi "google.golang.org/api/gmail/v1"
	peopleapi "google.golang.org/api/people/v1"
)

const (
	manualAuthTimeout   = 10
	callbackSuccessPage = `<html><body style="font-family:sans-serif;padding:2em"><h2>✓ Authentication successful</h2><p>You can close this tab and return to the terminal.</p></body></html>`
)

var scopes = []string{
	googleapi.GmailReadonlyScope,
	googleapi.GmailModifyScope,
	googleapi.GmailSendScope,
	peopleapi.ContactsScope,
	"https://www.googleapis.com/auth/userinfo.profile",
	"https://www.googleapis.com/auth/userinfo.email",
}

// fetchProfile fetches email, photo URL and display name from the Google userinfo endpoint.
func fetchProfile(ctx context.Context, client *http.Client) (email, photoURL, displayName string, err error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://www.googleapis.com/oauth2/v3/userinfo", nil)
	if err != nil {
		return "", "", "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	var info struct {
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.UnmarshalRead(resp.Body, &info); err != nil {
		return "", "", "", err
	}
	if info.Email == "" {
		return "", "", "", fmt.Errorf("empty email in userinfo response")
	}
	return info.Email, info.Picture, info.Name, nil
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
