package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

/*
Example YAML configuration file (frkr.yaml):

auth:
  client_id: "your-client-id"          # Required: OIDC Client ID
  auth_domain: "https://auth.frkr.io"  # Optional: OIDC Issuer/Domain
  audience: "https://api.frkr.io"      # Optional: JWT Audience
  callback_port: "38911"                # Optional: Local port for callback
*/

// AuthConfig holds OIDC authentication settings.
type AuthConfig struct {
	AuthDomain   string `yaml:"auth_domain"`
	ClientID     string `yaml:"client_id"`
	Audience     string `yaml:"audience"`
	CallbackPort string `yaml:"callback_port"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
}

// ClientConfig represents the global configuration for the CLI.
type ClientConfig struct {
	Auth AuthConfig `yaml:"auth"`
}

var (
	// Default values, can be overridden by ldflags or flags
	defaultAuthDomain   = "https://auth.frkr.io"
	defaultClientID     = ""
	defaultAudience     = "https://api.frkr.io"
	defaultCallbackPort = "38911"
	defaultConfigPath   = "frkr.yaml"

	// Runtime configuration variables, populated by init() and resolveLoginConfig()
	clientCfg ClientConfig
	configPath string
)

func init() {
	// Initialize with defaults
	clientCfg = ClientConfig{
		Auth: AuthConfig{
			AuthDomain:   defaultAuthDomain,
			ClientID:     defaultClientID,
			Audience:     defaultAudience,
			CallbackPort: defaultCallbackPort,
		},
	}

	// Register flags bound to the clientCfg struct fields
	loginCmd.Flags().StringVar(&clientCfg.Auth.AuthDomain, "auth-domain", defaultAuthDomain, "OIDC Auth Domain")
	loginCmd.Flags().StringVar(&clientCfg.Auth.ClientID, "client-id", defaultClientID, "OIDC Client ID")
	loginCmd.Flags().StringVar(&clientCfg.Auth.Audience, "audience", defaultAudience, "OIDC Audience")
	loginCmd.Flags().StringVar(&clientCfg.Auth.CallbackPort, "callback-port", defaultCallbackPort, "Local callback port")
	loginCmd.Flags().StringVar(&clientCfg.Auth.Username, "username", "", "Basic Auth Username")
	loginCmd.Flags().StringVar(&clientCfg.Auth.Password, "password", "", "Basic Auth Password")
	loginCmd.Flags().StringVar(&configPath, "config", "", "Path to YAML config file (mutually exclusive with other flags)")
}

// resolveLoginConfig handles the precedence logic:
// 1. If --config is passed, load from file. Error if other flags are set.
// 2. If no flags are set, try loading default config file.
// 3. Otherwise, use flags/defaults.
func resolveLoginConfig(cmd *cobra.Command) error {
	// Check if any auth-related flags were explicitly changed by the user
	flagsSet := cmd.Flags().Changed("auth-domain") ||
		cmd.Flags().Changed("client-id") ||
		cmd.Flags().Changed("audience") ||
		cmd.Flags().Changed("callback-port") ||
		cmd.Flags().Changed("username") ||
		cmd.Flags().Changed("password")

	if configPath != "" {
		// Case 1: Explicit config file
		if flagsSet {
			return fmt.Errorf("--config is mutually exclusive with other auth flags")
		}
		if err := loadConfigFromFile(configPath); err != nil {
			return fmt.Errorf("failed to load config from %s: %w", configPath, err)
		}
	} else if !flagsSet {
		// Case 2: No flags set, look for default file
		if _, err := os.Stat(defaultConfigPath); err == nil {
			// File exists, use it
			if err := loadConfigFromFile(defaultConfigPath); err != nil {
				return fmt.Errorf("failed to load default config %s: %w", defaultConfigPath, err)
			}
			fmt.Printf("Loaded configuration from %s\n", defaultConfigPath)
		}
	}
	
	// Case 3: Flags set or no file found -> stick with current clientCfg (which has flags/defaults)
	
	// Final Validation
	// If Basic Auth is provided, we don't need OIDC Client ID
	if clientCfg.Auth.Username != "" || clientCfg.Auth.Password != "" {
		if clientCfg.Auth.Username == "" || clientCfg.Auth.Password == "" {
			return fmt.Errorf("both --username and --password are required for basic auth")
		}
		// Basic auth is valid, proceed
		return nil
	}

	// Otherwise, OIDC is required
	if clientCfg.Auth.ClientID == "" {
		return fmt.Errorf("client-id is required (use --client-id flag, config file, or build with -ldflags)")
	}

	return nil
}

func loadConfigFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Unmarshal into a temporary struct to differentiate empty vs missing
	var cfg ClientConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}

	// Apply values from file, overwriting defaults
	if cfg.Auth.AuthDomain != "" {
		clientCfg.Auth.AuthDomain = cfg.Auth.AuthDomain
	}
	if cfg.Auth.ClientID != "" {
		clientCfg.Auth.ClientID = cfg.Auth.ClientID
	}
	if cfg.Auth.Audience != "" {
		clientCfg.Auth.Audience = cfg.Auth.Audience
	}
	if cfg.Auth.CallbackPort != "" {
		clientCfg.Auth.CallbackPort = cfg.Auth.CallbackPort
	}
	if cfg.Auth.Username != "" {
		clientCfg.Auth.Username = cfg.Auth.Username
	}
	if cfg.Auth.Password != "" {
		clientCfg.Auth.Password = cfg.Auth.Password
	}
	return nil
}
