package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to frkr",
	Long:  `Authenticate with the frkr platform using OIDC (PKCE flow).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Ensure we have necessary config to login
		if err := validateConfig(); err != nil {
			return err
		}

		return login()
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)

	// Register flags bounds to the authConfig global
	loginCmd.Flags().StringVar(&authConfig.AuthDomain, "auth-domain", defaultAuthDomain, "OIDC Auth Domain")
	loginCmd.Flags().StringVar(&authConfig.ClientID, "client-id", defaultClientID, "OIDC Client ID")
	loginCmd.Flags().StringVar(&authConfig.Audience, "audience", defaultAudience, "OIDC Audience")
	loginCmd.Flags().StringVar(&authConfig.CallbackPort, "callback-port", defaultCallbackPort, "Local callback port")
	loginCmd.Flags().StringVar(&authConfig.Username, "username", "", "Basic Auth Username")
	loginCmd.Flags().StringVar(&authConfig.Password, "password", "", "Basic Auth Password")
}

// validateConfig ensures that we have enough configuration to proceed with a login.
func validateConfig() error {
	// If Basic Auth is provided, we don't need OIDC Client ID
	if authConfig.Username != "" || authConfig.Password != "" {
		if authConfig.Username == "" || authConfig.Password == "" {
			return fmt.Errorf("both --username and --password are required for basic auth")
		}
		// Basic auth is valid, proceed
		return nil
	}

	// Otherwise, OIDC is required
	if authConfig.ClientID == "" {
		return fmt.Errorf("client-id is required (use --client-id flag or build with -ldflags)")
	}

	return nil
}

func login() error {
	// Always overwrite existing login session (Last Write Wins)
	store, err := loadAuthStore()
	if err != nil || store == nil {
		store = &AuthTokenStore{}
	}
	
	manager := NewAuthManager(store)
	ctx := context.Background()

	// 0. Check for Basic Auth
	if authConfig.Username != "" && authConfig.Password != "" {
		fmt.Printf("Logging in with Basic Auth user: %s\n", authConfig.Username)
		
		store.ClearOIDC()
		store.BasicAuthUsername = authConfig.Username
		store.BasicAuthPassword = authConfig.Password
		
		if err := saveAuthStore(store); err != nil {
			return fmt.Errorf("failed to save auth credentials: %w", err)
		}
		
		fmt.Println("✅ Successfully logged in (Basic Auth Credentials Saved)!")
		return nil
	}

	// 1. Interactive OIDC Login via AuthManager
	return manager.InteractiveLogin(ctx)
}
