package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/pkg/browser"
	"golang.org/x/oauth2"
)

// AuthConfig holds OIDC authentication settings.
type AuthConfig struct {
	AuthDomain   string
	ClientID     string
	Audience     string
	CallbackPort string
	Username     string
	Password     string
}

var (
	// Default values
	defaultAuthDomain   = "https://auth.frkr.io"
	defaultClientID     = "" // Set via build flags or runtime flag
	defaultAudience     = "https://api.frkr.io"
	defaultCallbackPort = "38911"

	// Global auth configuration
	authConfig AuthConfig
)

func init() {
	// Initialize with defaults
	authConfig = AuthConfig{
		AuthDomain:   defaultAuthDomain,
		ClientID:     defaultClientID,
		Audience:     defaultAudience,
		CallbackPort: defaultCallbackPort,
	}
}

// AuthManager handles authentication logic
type AuthManager struct {
	Store *AuthTokenStore
}

// NewAuthManager creates a new AuthManager with the given store
func NewAuthManager(store *AuthTokenStore) *AuthManager {
	return &AuthManager{Store: store}
}

// GetAuthHeader returns the Authorization header value (e.g. "Bearer ...")
// It handles refresh and interactive login if allowed.
// forceOAuth: ignore basic auth and force OIDC.
func (m *AuthManager) GetAuthHeader(ctx context.Context, allowInteractive bool, forceOAuth bool) (string, error) {
	// 1. Check Basic Auth (unless forced to use OAuth)
	if !forceOAuth && m.Store.BasicAuthUsername != "" && m.Store.BasicAuthPassword != "" {
		return "Basic " + basicAuth(m.Store.BasicAuthUsername, m.Store.BasicAuthPassword), nil
	}

	// 2. OIDC Logic
	conf := &oauth2.Config{
		ClientID: authConfig.ClientID,
		Endpoint: oauth2.Endpoint{
			AuthURL:  authConfig.AuthDomain + "/authorize",
			TokenURL: authConfig.AuthDomain + "/oauth/token",
		},
		RedirectURL: "http://localhost:" + authConfig.CallbackPort + "/callback",
		Scopes:      []string{"openid", "profile", "email", "offline_access"},
	}

	// Construct Token source from store
	storedToken := m.Store.ToToken()
	
	// If we have no token and no refresh token, and interactive is allowed, trigger login
	if storedToken.AccessToken == "" && storedToken.RefreshToken == "" {
		if allowInteractive {
			// Trigger Login
			// Note: We need to update the store after login
			if err := login(); err != nil { 
				return "", fmt.Errorf("login failed: %w", err)
			}
			// Reload store
			newStore, err := loadAuthStore()
			if err != nil {
				return "", err
			}
			m.Store = newStore
			return "Bearer " + m.Store.AccessToken, nil
		}
		return "", fmt.Errorf("authentication required")
	}

	// We have a token (or refresh token). Use oauth2 package to refresh if needed.
	// ReuseTokenSource will use the existing token if valid, or call source.Token() (refresh) if expired.
	tokenSource := conf.TokenSource(ctx, storedToken)
	
	newToken, err := tokenSource.Token()
	if err != nil {
		// Refresh failed.
		if allowInteractive {
			fmt.Println("🔄 Session expired, please log in again...")
			if err := login(); err != nil {
				return "", fmt.Errorf("re-login failed: %w", err)
			}
			newStore, err := loadAuthStore()
			if err != nil {
				return "", err
			}
			m.Store = newStore
			return "Bearer " + m.Store.AccessToken, nil
		}
		return "", fmt.Errorf("session expired and interactive login disabled: %w", err)
	}

	// If token refreshed (changed), save it
	if newToken.AccessToken != storedToken.AccessToken {
		// Update store
		m.Store.UpdateFromToken(newToken)
		if err := saveAuthStore(m.Store); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save refreshed token: %v\n", err)
		}
	}

	return "Bearer " + newToken.AccessToken, nil
}

// InteractiveLogin performs the OIDC Authorization Code Flow with PKCE
func (m *AuthManager) InteractiveLogin(ctx context.Context) error {
	// 1. Configure OIDC
	conf := &oauth2.Config{
		ClientID: authConfig.ClientID,
		Endpoint: oauth2.Endpoint{
			AuthURL:  authConfig.AuthDomain + "/authorize",
			TokenURL: authConfig.AuthDomain + "/oauth/token",
		},
		RedirectURL: "http://localhost:" + authConfig.CallbackPort + "/callback",
		Scopes:      []string{"openid", "profile", "email", "offline_access"},
	}

	// 2. Generate PKCE Verifier and Challenge
	verifier := generateRandomString(64)
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])

	// 3. Setup local server
	codeChan := make(chan string)
	errChan := make(chan error)
	server := &http.Server{Addr: ":" + authConfig.CallbackPort}

	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("no code received")
			http.Error(w, "Login failed: No code received", http.StatusBadRequest)
			return
		}
		codeChan <- code
		fmt.Fprint(w, "<h1>Login Successful</h1><p>You can close this window and return to the CLI.</p>")
	})

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// 4. Open Browser
	url := conf.AuthCodeURL("state-token",
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("audience", authConfig.Audience),
	)

	fmt.Printf("Opening browser to login: %s\n", url)
	if err := browser.OpenURL(url); err != nil {
		fmt.Printf("Failed to open browser: %v\n", err)
		fmt.Println("Please copy and paste the URL above into your browser.")
	}

	// 5. Wait for Code
	var code string
	select {
	case code = <-codeChan:
	case err := <-errChan:
		return fmt.Errorf("callback server error: %w", err)
	case <-time.After(5 * time.Minute):
		return fmt.Errorf("login timed out")
	}

	// Shutdown server
	server.Shutdown(ctx)

	// 6. Exchange Code for Token
	fmt.Println("Exchanging code for token...")
	token, err := conf.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", verifier))
	if err != nil {
		return fmt.Errorf("failed to exchange token: %w", err)
	}

	// 7. Save State (Clear Basic Auth, Set OIDC)
	m.Store.ClearBasicAuth()
	m.Store.UpdateFromToken(token)

	if err := saveAuthStore(m.Store); err != nil {
		return fmt.Errorf("failed to save auth state: %w", err)
	}

	fmt.Println("✅ Successfully logged in!")
	return nil
}

func generateRandomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// basicAuth returns the base64 encoded username:password
func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}
