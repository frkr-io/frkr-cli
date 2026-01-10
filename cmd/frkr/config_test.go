package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestResolveLoginConfig(t *testing.T) {
	// Setup temporary directory for test files
	tempDir, err := os.MkdirTemp("", "frkr-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save original global state
	origDefaultAuthDomain := defaultAuthDomain
	origDefaultClientID := defaultClientID
	origDefaultAudience := defaultAudience
	origDefaultCallbackPort := defaultCallbackPort
	origConfigPath := configPath
	origDefaultConfigPath := defaultConfigPath
	origClientCfg := clientCfg

	defer func() {
		// Restore global state
		defaultAuthDomain = origDefaultAuthDomain
		defaultClientID = origDefaultClientID
		defaultAudience = origDefaultAudience
		defaultCallbackPort = origDefaultCallbackPort
		configPath = origConfigPath
		defaultConfigPath = origDefaultConfigPath
		clientCfg = origClientCfg
	}()

	tests := []struct {
		name          string
		flags         map[string]string
		configFile    string
		configContent string
		wantErr       bool
		expectedID    string
	}{
		{
			name: "Explicit Config File",
			flags: map[string]string{
				"config": "custom.yaml",
			},
			configFile: "custom.yaml",
			configContent: `
auth:
  client_id: "from-yaml"
  auth_domain: "https://yaml.com"
`,
			expectedID: "from-yaml",
			wantErr:    false,
		},
		{
			name: "Flag Override (No Config File)",
			flags: map[string]string{
				"client-id": "from-flag",
			},
			expectedID: "from-flag",
			wantErr:    false,
		},
		{
			name: "Default Config File (Implicit)",
			flags: map[string]string{},
			configFile: defaultConfigPath, // Use the default name
			configContent: `
auth:
  client_id: "from-default-yaml"
`,
			expectedID: "from-default-yaml",
			wantErr:    false,
		},
		{
			name: "Conflict: Config Flag + Auth Flag",
			flags: map[string]string{
				"config":    "custom.yaml",
				"client-id": "conflict",
			},
			configFile: "custom.yaml",
			configContent: `
auth:
  client_id: "from-yaml"
`,
			wantErr: true,
		},
		{
			name:    "Missing Client ID",
			flags:   map[string]string{}, // No flags, no config file
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset globals for each test
			clientCfg = ClientConfig{
				Auth: AuthConfig{
					AuthDomain:   defaultAuthDomain,
					ClientID:     defaultClientID,
					Audience:     defaultAudience,
					CallbackPort: defaultCallbackPort,
				},
			}
			
			// Reset paths to original values
			configPath = ""
			defaultConfigPath = origDefaultConfigPath

			// Create dummy command to bind flags
			cmd := &cobra.Command{}
			
			cmd.Flags().StringVar(&clientCfg.Auth.AuthDomain, "auth-domain", defaultAuthDomain, "")
			cmd.Flags().StringVar(&clientCfg.Auth.ClientID, "client-id", defaultClientID, "")
			cmd.Flags().StringVar(&clientCfg.Auth.Audience, "audience", defaultAudience, "")
			cmd.Flags().StringVar(&clientCfg.Auth.CallbackPort, "callback-port", defaultCallbackPort, "")
			cmd.Flags().StringVar(&configPath, "config", "", "")

			// Create config file if needed
			if tt.configFile != "" {
				fullPath := filepath.Join(tempDir, tt.configFile)
				if err := os.WriteFile(fullPath, []byte(tt.configContent), 0644); err != nil {
					t.Fatalf("Failed to write config file: %v", err)
				}
				
				// If using custom config, update the flag or global path
				if _, ok := tt.flags["config"]; ok {
					// Update the 'config' variable that `resolveLoginConfig` reads
					configPath = fullPath
					// Also mark flag as changed
					cmd.Flags().Set("config", fullPath)
				} else if tt.configFile == defaultConfigPath {
					// We need to point the global default path to our temp dir version
					defaultConfigPath = fullPath
				}
			} else {
				// Ensure default path doesn't exist/conflict
				defaultConfigPath = filepath.Join(tempDir, "non-existent.yaml")
				configPath = ""
			}

			// Apply other flags
			for k, v := range tt.flags {
				if k != "config" {
					cmd.Flags().Set(k, v)
					// Manually set into the struct because cobra parsing isn't happening here
					switch k {
					case "client-id":
						clientCfg.Auth.ClientID = v
					case "auth-domain":
						clientCfg.Auth.AuthDomain = v
					case "audience":
						clientCfg.Auth.Audience = v
					case "callback-port":
						clientCfg.Auth.CallbackPort = v
					}
				}
			}

			err := resolveLoginConfig(cmd)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveLoginConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && clientCfg.Auth.ClientID != tt.expectedID {
				t.Errorf("expected clientID %q, got %q", tt.expectedID, clientCfg.Auth.ClientID)
			}
		})
	}
}
