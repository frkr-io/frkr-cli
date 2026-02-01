//go:build integration

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIntegration_AuthFlows(t *testing.T) {
	// Setup Temp Home
	tempHome, err := os.MkdirTemp("", "frkr-integration")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempHome)
	
	// Mock OIDC Server
	// We need a server that returns tokens
	server := &http.Server{Addr: ":39090", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token":  "mock-access-token",
				"refresh_token": "mock-refresh-token",
				"expires_in":    3600,
				"token_type":    "Bearer",
			})
			return
		}
		if r.URL.Path == "/authorize" {
			// Redirect back to callback
			redirectURI := r.URL.Query().Get("redirect_uri")
			state := r.URL.Query().Get("state")
			http.Redirect(w, r, redirectURI+"?code=mock-code&state="+state, http.StatusFound)
			return
		}
	})}
	
	go server.ListenAndServe()
	defer server.Close()
	
	// Override Home Dir for the test process? 
	// The `getAuthDir` uses `os.UserHomeDir()`. We can't easily change that for the current process safely in parallel tests.
	// But `getAuthDir` is in `auth_store.go`. We could make it overridable.
	// OR we just assume we can set env var "HOME".
	os.Setenv("HOME", tempHome)
	
	// Initialize Config Global
	authConfig = AuthConfig{
		AuthDomain:   "http://localhost:39090",
		ClientID:     "test-client",
		Audience:     "test-audience",
		CallbackPort: "38911",
	}

	// Helper to reset store
	resetStore := func() {
		os.Remove(filepath.Join(tempHome, ".frkr", "auth.json"))
	}

	t.Run("Scenario: Basic Auth Overwrites Store", func(t *testing.T) {
		resetStore()
		
		// 1. Save some garbage OIDC to verify overwrite
		store := &AuthTokenStore{AccessToken: "garbage"}
		saveAuthStore(store)
		
		// 2. Perform Login with Basic Auth
		// Simulate "frkr stream --username=user --password=pass"
		store, _ = loadAuthStore()
		if store == nil { store = &AuthTokenStore{} }
		
		store.ClearOIDC()
		store.BasicAuthUsername = "user"
		store.BasicAuthPassword = "pass"
		saveAuthStore(store)
		
		// Verify
		loaded, _ := loadAuthStore()
		if loaded.AccessToken != "" {
			t.Error("OIDC token should be cleared")
		}
		if loaded.BasicAuthUsername != "user" {
			t.Error("Basic Auth username not saved")
		}
	})

	t.Run("Scenario: OAuth Flag Overwrites Store", func(t *testing.T) {
		resetStore()
		
		// 1. Save Basic Auth
		store := &AuthTokenStore{BasicAuthUsername: "old-user", BasicAuthPassword: "old-pass"}
		saveAuthStore(store)
		
		// 2. Simulate "frkr stream --oauth"
		store, _ = loadAuthStore()
		if store.BasicAuthUsername != "" {
			store.ClearBasicAuth()
			saveAuthStore(store)
		}
		
		// Verify
		loaded, _ := loadAuthStore()
		if loaded.BasicAuthUsername != "" {
			t.Error("Basic Auth should be cleared")
		}
	})
	


	t.Run("Scenario: AuthManager Auto-Refresh", func(t *testing.T) {
		resetStore()
		
		// 1. Seed Store with Expired Token
		store := &AuthTokenStore{
			AccessToken:  "expired",
			RefreshToken: "valid-refresh",
			Expiry:       time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		}
		
		manager := NewAuthManager(store)
		
		// 2. Call GetAuthHeader
		// allowInteractive=false to force refresh path
		header, err := manager.GetAuthHeader(context.Background(), false, false)
		if err != nil {
			t.Fatalf("GetAuthHeader failed: %v", err)
		}
		
		if header != "Bearer mock-access-token" {
			t.Errorf("Expected refreshed token, got: %s", header)
		}
		
		// 3. Verify Store Updated
		if manager.Store.AccessToken != "mock-access-token" {
			t.Error("Store not updated with new token")
		}
	})
}
