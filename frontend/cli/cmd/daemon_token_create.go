package cmd

import (
	"fmt"
	"os"
	"time"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/durationpb"
)

type tokenCreateOptions struct {
	Description   string
	Expires       string
	RenderOptions RenderOptions
}

func NewDaemonTokenCreateCmd() *cobra.Command {
	var options tokenCreateOptions

	cmd := &cobra.Command{
		Use:   "create <name> [flags]",
		Short: "Generate a new API token for remote daemon authentication",
		Args:  cobra.ExactArgs(1),
		Long: `Generate a new API token for remote daemon authentication.

The token is displayed once and cannot be retrieved again. Store it securely
in a password manager or system keyring.

Tokens are used to authenticate CLI commands against remote daemon instances
over HTTPS. Configure a context with the token using 'construct context add'.`,
		Example: `  # Create token with default 90-day expiry
  construct daemon token create laptop-token

  # Create token with custom expiry and description
  construct daemon token create ci-pipeline \
    --description "GitHub Actions pipeline token" \
    --expires 30d

  # Create token with JSON output for scripting
  construct daemon token create automation --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			expiresDuration, err := ParseDuration(options.Expires)
			if err != nil {
				return fmt.Errorf("invalid expiry duration: %w", err)
			}

			if err := ValidateTokenExpiry(expiresDuration); err != nil {
				return err
			}

			client := getAPIClient(cmd.Context())

			req := &connect.Request[v1.CreateTokenRequest]{
				Msg: &v1.CreateTokenRequest{
					Name:      name,
					ExpiresIn: durationpb.New(expiresDuration),
				},
			}

			if options.Description != "" {
				req.Msg.Description = &options.Description
			}

			resp, err := client.Auth().CreateToken(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("failed to create token: %w", err)
			}

			display := &TokenCreateDisplay{
				Name:      name,
				Token:     resp.Msg.Token,
				ExpiresAt: resp.Msg.ExpiresAt.AsTime().Format(time.RFC3339),
			}

			if err := getRenderer(cmd.Context()).Render(display, &options.RenderOptions); err != nil {
				return err
			}

			if options.RenderOptions.Format == OutputFormatCard || options.RenderOptions.Format == "" {
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintln(os.Stderr, "⚠️  Save this token securely - it cannot be retrieved again.")
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&options.Description, "description", "", "Optional description of token purpose")
	cmd.Flags().StringVar(&options.Expires, "expires", "90d", "Token lifetime (default: 90d, max: 365d)")
	addRenderOptions(cmd, &options.RenderOptions)
	WithCardFormat(&options.RenderOptions)

	return cmd
}
