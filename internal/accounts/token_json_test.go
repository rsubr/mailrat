package accounts

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// Tokens already in users' databases were written by encoding/json v1; they
// must keep loading under json/v2.
func TestTokenFromJSONReadsLegacyV1(t *testing.T) {
	legacy := `{"access_token":"ya29.a","token_type":"Bearer","refresh_token":"1//r","expiry":"2026-01-02T03:04:05.123456789Z","expires_in":3599}`
	tok, err := tokenFromJSON(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "ya29.a" || tok.RefreshToken != "1//r" || tok.TokenType != "Bearer" || tok.ExpiresIn != 3599 {
		t.Fatalf("%+v", tok)
	}
	if want := time.Date(2026, 1, 2, 3, 4, 5, 123456789, time.UTC); !tok.Expiry.Equal(want) {
		t.Fatalf("expiry %v", tok.Expiry)
	}
}

func TestTokenFromJSONLegacyZeroExpiryAndExtraFields(t *testing.T) {
	// v1 wrote a zero expiry as year 1, and old rows may carry unknown keys.
	tok, err := tokenFromJSON(`{"access_token":"a","refresh_token":"r","expiry":"0001-01-01T00:00:00Z","id_token":"x"}`)
	if err != nil || !tok.Expiry.IsZero() || tok.AccessToken != "a" {
		t.Fatalf("%+v %v", tok, err)
	}
}

func TestTokenJSONRoundTripAndV1Compatible(t *testing.T) {
	in := &oauth2.Token{AccessToken: "a", TokenType: "Bearer", RefreshToken: "r", Expiry: time.Now().Add(time.Hour).Round(time.Second).UTC()}
	s, err := tokenToJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := tokenFromJSON(s)
	if err != nil || out.AccessToken != "a" || out.RefreshToken != "r" || !out.Expiry.Equal(in.Expiry) {
		t.Fatalf("round trip: %+v %v", out, err)
	}
	// Rolling back to a v1 build must still read what v2 wrote.
	var v1 oauth2.Token
	if err := json.Unmarshal([]byte(s), &v1); err != nil || v1.RefreshToken != "r" || !v1.Expiry.Equal(in.Expiry) {
		t.Fatalf("v1 read of v2 output: %s %v", s, err)
	}
	if !strings.Contains(s, `"access_token":"a"`) {
		t.Fatalf("key names changed: %s", s)
	}
}

func TestTokenFromJSONRejectsGarbage(t *testing.T) {
	for _, s := range []string{"", "{", `{"access_token":1}`, `{"access_token":"a","access_token":"b"}`} {
		if _, err := tokenFromJSON(s); err == nil {
			t.Errorf("want error for %q", s)
		}
	}
}
