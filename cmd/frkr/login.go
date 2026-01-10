package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to frkr",
	Long:  `Authenticate with the frkr platform using OIDC (PKCE flow).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Calculate config resolution here
		if err := resolveLoginConfig(cmd); err != nil {
			return err
		}

		return login()
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)
}

func login() error {
	// Check if already logged in (skip check if force login is requested? No, existing behavior is fine)
	store, err := loadAuthStore()
	if err == nil && store != nil && store.IsValid() {
		fmt.Println("Already logged in.")
		return nil
	}

	ctx := context.Background()

	// 0. Check for Basic Auth
	if clientCfg.Auth.Username != "" && clientCfg.Auth.Password != "" {
		fmt.Printf("Logging in with Basic Auth user: %s\n", clientCfg.Auth.Username)
		
		// For Basic Auth, we generally don't "validate" against an IdP proactively in this CLI design,
		// we just save the credentials to the store so subsequent commands use them.
		// Alternatively, we could try to call a "whoami" endpoint if one existed, but per specs we just save.
		
		store = &AuthTokenStore{
			BasicAuthUsername: clientCfg.Auth.Username,
			BasicAuthPassword: clientCfg.Auth.Password,
		}
		
		if err := saveAuthStore(store); err != nil {
			return fmt.Errorf("failed to save auth credentials: %w", err)
		}
		
		fmt.Println("✅ Successfully logged in (Basic Auth Credentials Saved)!")
		return nil
	}

	// 1. Configure OIDC
	conf := &oauth2.Config{
		ClientID: clientCfg.Auth.ClientID,
		Endpoint: oauth2.Endpoint{
			AuthURL:  clientCfg.Auth.AuthDomain + "/authorize",
			TokenURL: clientCfg.Auth.AuthDomain + "/oauth/token",
		},
		RedirectURL: "http://localhost:" + clientCfg.Auth.CallbackPort + "/callback",
		Scopes:      []string{"openid", "profile", "email", "offline_access"},
	}

	// 2. Generate PKCE Verifier and Challenge
	verifier := generateRandomString(64)
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])

	// 3. Setup local server
	codeChan := make(chan string)
	errChan := make(chan error)
	server := &http.Server{Addr: ":" + clientCfg.Auth.CallbackPort}

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
		oauth2.SetAuthURLParam("audience", clientCfg.Auth.Audience),
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

	// 7. Save State
	store = &AuthTokenStore{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry.Format(time.RFC3339),
	}

	if err := saveAuthStore(store); err != nil {
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
