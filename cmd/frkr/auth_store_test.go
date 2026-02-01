package main

import (
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestAuthTokenStore_ToToken(t *testing.T) {
	expiry := time.Now().Add(1 * time.Hour).Round(time.Second)
	store := &AuthTokenStore{
		AccessToken:  "access-123",
		RefreshToken: "refresh-123",
		Expiry:       expiry.Format(time.RFC3339),
	}

	token := store.ToToken()

	if token.AccessToken != "access-123" {
		t.Errorf("expected AccessToken 'access-123', got %s", token.AccessToken)
	}
	if token.RefreshToken != "refresh-123" {
		t.Errorf("expected RefreshToken 'refresh-123', got %s", token.RefreshToken)
	}
	if !token.Expiry.Equal(expiry) {
		t.Errorf("expected Expiry %v, got %v", expiry, token.Expiry)
	}
}

func TestAuthTokenStore_UpdateFromToken(t *testing.T) {
	expiry := time.Now().Add(2 * time.Hour).Round(time.Second)
	token := &oauth2.Token{
		AccessToken:  "new-access",
		RefreshToken: "new-refresh",
		Expiry:       expiry,
	}

	store := &AuthTokenStore{}
	store.UpdateFromToken(token)

	if store.AccessToken != "new-access" {
		t.Errorf("expected AccessToken 'new-access', got %s", store.AccessToken)
	}
	if store.RefreshToken != "new-refresh" {
		t.Errorf("expected RefreshToken 'new-refresh', got %s", store.RefreshToken)
	}
	
	parsedExpiry, _ := time.Parse(time.RFC3339, store.Expiry)
	if !parsedExpiry.Equal(expiry) {
		t.Errorf("expected Expiry %v, got %v", expiry, parsedExpiry)
	}
}

func TestAuthTokenStore_ClearMethods(t *testing.T) {
	store := &AuthTokenStore{
		AccessToken:       "oidc",
		BasicAuthUsername: "basic",
	}

	store.ClearBasicAuth()
	if store.BasicAuthUsername != "" {
		t.Error("expected BasicAuthUsername to be empty")
	}
	if store.AccessToken != "oidc" {
		t.Error("expected AccessToken to remain set")
	}

	store.ClearOIDC()
	if store.AccessToken != "" {
		t.Error("expected AccessToken to be empty")
	}
}
