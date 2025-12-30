package cmd

import (
	"fmt"
	"os"
	"strings"

	api "github.com/furisto/construct/api/go/client"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type contextAddOptions struct {
	Endpoint   string
	Kind       string
	AuthToken  bool
	SetCurrent bool
}

func NewContextAddCmd() *cobra.Command {
	var options contextAddOptions

	cmd := &cobra.Command{
		Use:   "add <name> [flags]",
		Short: "Add or update a context",
		Args:  cobra.ExactArgs(1),
		Long: `Add a new context or update an existing one.

The endpoint can be a Unix socket path or HTTP(S) URL. The connection kind 
is auto-detected from the endpoint format if not explicitly specified:
  • Paths starting with / are treated as Unix sockets (kind: unix)
  • URLs are treated as HTTP connections (kind: http)

Authentication tokens are securely stored in the system keyring (macOS Keychain, 
Linux Secret Service, Windows Credential Manager).`,
		Example: `  # Add local Unix socket context
  construct context add local --endpoint /home/user/.construct/construct.sock

  # Add remote HTTP context with authentication
  construct context add production \
    --endpoint https://construct.prod.example.com:8443 \
    --auth-token \
    --set-current

  # Update existing context endpoint
  construct context add staging --endpoint https://new-staging.example.com:8443`,
		RunE: func(cmd *cobra.Command, args []string) error {
			contextName := args[0]
			contextManager := getContextManager(cmd.Context())

			kind := options.Kind
			if kind == "" {
				if strings.HasPrefix(options.Endpoint, "/") {
					kind = "unix"
				} else {
					kind = "http"
				}
			}

			var authConfig *api.AuthConfig
			if options.AuthToken {
				fmt.Fprint(cmd.OutOrStdout(), "Enter authentication token: ")
				tokenBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Fprintln(cmd.OutOrStdout())
				if err != nil {
					return fmt.Errorf("failed to read token: %w", err)
				}

				token := strings.TrimSpace(string(tokenBytes))
				if token == "" {
					return fmt.Errorf("token cannot be empty")
				}

				authConfig = &api.AuthConfig{
					Type:     api.AuthTypeToken,
					TokenRef: api.KeyringRefPrefix + contextName,
				}

				if err := contextManager.StoreToken(contextName, token); err != nil {
					return fmt.Errorf("failed to store token in keyring: %w", err)
				}
			}

			existed, err := contextManager.UpsertContext(contextName, kind, options.Endpoint, options.SetCurrent, authConfig)
			if err != nil {
				return err
			}

			action := "created"
			if existed {
				action = "updated"
			}

			message := fmt.Sprintf("Context %q %s", contextName, action)
			if options.SetCurrent {
				message += " and set as current"
			}
			fmt.Fprintln(cmd.OutOrStdout(), message)

			return nil
		},
	}

	cmd.Flags().StringVar(&options.Endpoint, "endpoint", "", "Daemon endpoint (Unix socket path or HTTP URL)")
	cmd.Flags().StringVar(&options.Kind, "kind", "", "Connection type (unix or http, auto-detected if not specified)")
	cmd.Flags().BoolVar(&options.AuthToken, "auth-token", false, "Prompt for authentication token")
	cmd.Flags().BoolVar(&options.SetCurrent, "set-current", false, "Set this context as current")

	cmd.MarkFlagRequired("endpoint")

	return cmd
}
