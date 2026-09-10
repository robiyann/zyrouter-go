package admin

import "testing"

func TestValidateProviderConnectionCredentials(t *testing.T) {
	if err := validateProviderConnectionCredentials("oauth", `{"apiKey":"","accessToken":""}`); err == nil {
		t.Fatal("expected empty OAuth credentials to be rejected")
	}
	if err := validateProviderConnectionCredentials("oauth", `{"accessToken":"workos:token"}`); err != nil {
		t.Fatalf("expected valid OAuth credentials, got %v", err)
	}
	if err := validateProviderConnectionCredentials("apikey", `{"apiKey":""}`); err != nil {
		t.Fatalf("API-key validation should remain unchanged, got %v", err)
	}
}
