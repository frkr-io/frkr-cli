package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"
)

// AuthTokenStore holds the persisted authentication token (OIDC) or credentials (Basic).
type AuthTokenStore struct {
	// OIDC Fields
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Expiry       string `json:"expiry,omitempty"`

	// Basic Auth Fields
	BasicAuthUsername string `json:"basic_auth_username,omitempty"`
	BasicAuthPassword string `json:"basic_auth_password,omitempty"`
}

// ClearBasicAuth clears basic auth credentials
func (s *AuthTokenStore) ClearBasicAuth() {
	s.BasicAuthUsername = ""
	s.BasicAuthPassword = ""
}

// ClearOIDC clears OIDC tokens
func (s *AuthTokenStore) ClearOIDC() {
	s.AccessToken = ""
	s.RefreshToken = ""
	s.Expiry = ""
}

// ToToken converts the store to an oauth2.Token
func (s *AuthTokenStore) ToToken() *oauth2.Token {
	t := &oauth2.Token{
		AccessToken:  s.AccessToken,
		RefreshToken: s.RefreshToken,
		TokenType:    "Bearer",
	}

	if s.Expiry != "" {
		if expiry, err := time.Parse(time.RFC3339, s.Expiry); err == nil {
			t.Expiry = expiry
		}
	}
	return t
}

// UpdateFromToken updates the store from an oauth2.Token
func (s *AuthTokenStore) UpdateFromToken(t *oauth2.Token) {
	s.AccessToken = t.AccessToken
	s.RefreshToken = t.RefreshToken
	if !t.Expiry.IsZero() {
		s.Expiry = t.Expiry.Format(time.RFC3339)
	}
}

// IsValid checks if the token is present and not expired (with optional buffer),
// OR if valid basic auth credentials are present.
func (s *AuthTokenStore) IsValid() bool {
	// 1. Check Basic Auth (Preferred if present? Or should OIDC take precedence? Let's treat valid if either is good)
	if s.BasicAuthUsername != "" && s.BasicAuthPassword != "" {
		return true
	}

	// 2. Check OIDC
	if s.AccessToken == "" {
		return false
	}

	// If expiry is set, check it; otherwise assume good (or handle as needed)
	if s.Expiry != "" {
		expiry, err := time.Parse(time.RFC3339, s.Expiry)
		if err != nil {
			return false // Invalid format, treat as invalid
		}
		// Consider expired if it expires within the next 10 seconds
		if time.Now().Add(10 * time.Second).After(expiry) {
			return false
		}
	}
	return true
}

func getAuthDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".frkr"), nil
}

func getAuthPath() (string, error) {
	dir, err := getAuthDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "auth.json"), nil // Explicitly named auth.json
}

func loadAuthStore() (*AuthTokenStore, error) {
	path, err := getAuthPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &AuthTokenStore{}, nil
	}
	if err != nil {
		return nil, err
	}

	var store AuthTokenStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}

	return &store, nil
}

func saveAuthStore(store *AuthTokenStore) error {
	dir, err := getAuthDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path, err := getAuthPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600) // Secure permissions
}
