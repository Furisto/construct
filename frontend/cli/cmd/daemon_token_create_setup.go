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

type tokenCreateSetupOptions struct {
	CodeExpires   string
	TokenExpires  string
	RenderOptions RenderOptions
}

func NewDaemonTokenCreateSetupCmd() *cobra.Command {
	var options tokenCreateSetupOptions

	cmd := &cobra.Command{
		Use:   "create-setup <token-name> [flags]",
		Short: "Generate a short-lived setup code for secure token distribution",
		Args:  cobra.ExactArgs(1),
		Long: `Generate a short-lived setup code for secure token distribution.

Setup codes provide a secure way to distribute tokens to remote clients without
sending the token itself over insecure channels. The code expires quickly and
can only be used once.

The client exchanges the setup code for a token using:
  construct context add <name> --endpoint <url> --setup-code <code>`,
		Example: `  # Create setup code with default settings
  construct daemon token create-setup remote-laptop

  # Create setup code with custom expiry times
  construct daemon token create-setup staging-server \
    --code-expires 1h \
    --token-expires 30d

  # Create setup code with JSON output
  construct daemon token create-setup prod-api --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			tokenName := args[0]

			codeExpiresDuration, err := ParseDuration(options.CodeExpires)
			if err != nil {
				return fmt.Errorf("invalid code expiry duration: %w", err)
			}

			if err := ValidateSetupCodeExpiry(codeExpiresDuration); err != nil {
				return err
			}

			tokenExpiresDuration, err := ParseDuration(options.TokenExpires)
			if err != nil {
				return fmt.Errorf("invalid token expiry duration: %w", err)
			}

			if err := ValidateTokenExpiry(tokenExpiresDuration); err != nil {
				return err
			}

			client := getAPIClient(cmd.Context())

			req := &connect.Request[v1.CreateSetupCodeRequest]{
				Msg: &v1.CreateSetupCodeRequest{
					TokenName:      tokenName,
					ExpiresIn:      durationpb.New(codeExpiresDuration),
					TokenExpiresIn: durationpb.New(tokenExpiresDuration),
				},
			}

			resp, err := client.Auth().CreateSetupCode(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("failed to create setup code: %w", err)
			}

			display := &SetupCodeDisplay{
				TokenName: tokenName,
				SetupCode: resp.Msg.SetupCode,
				ExpiresAt: resp.Msg.ExpiresAt.AsTime().Format(time.RFC3339),
			}

			if err := getRenderer(cmd.Context()).Render(display, &options.RenderOptions); err != nil {
				return err
			}

			if options.RenderOptions.Format == OutputFormatCard || options.RenderOptions.Format == "" {
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintln(os.Stderr, "Share this code securely with the user. They can exchange it for a token using:")
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintf(os.Stderr, "  construct context add <name> \\\n")
				fmt.Fprintf(os.Stderr, "    --endpoint <daemon-url> \\\n")
				fmt.Fprintf(os.Stderr, "    --setup-code %s\n", resp.Msg.SetupCode)
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintln(os.Stderr, "⚠️  This code can only be used once and expires in", FormatRelativeTime(resp.Msg.ExpiresAt.AsTime()))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&options.CodeExpires, "code-expires", "5m", "Setup code lifetime (default: 5m, max: 72h)")
	cmd.Flags().StringVar(&options.TokenExpires, "token-expires", "90d", "Resulting token lifetime (default: 90d, max: 365d)")
	addRenderOptions(cmd, &options.RenderOptions)
	WithCardFormat(&options.RenderOptions)

	return cmd
}
