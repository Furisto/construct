package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type tokenRevokeOptions struct {
	Force bool
}

func NewDaemonTokenRevokeCmd() *cobra.Command {
	var options tokenRevokeOptions

	cmd := &cobra.Command{
		Use:   "revoke <id> [flags]",
		Short: "Revoke a token by ID",
		Args:  cobra.ExactArgs(1),
		Long: `Revoke a token by ID, preventing further use.

Once revoked, the token becomes invalid immediately and any systems using it
will lose access. Revocation is permanent and cannot be undone.`,
		Example: `  # Revoke token with confirmation prompt
  construct daemon token revoke b7f8c9d0-1234-5678-90ab-cdef12345678

  # Revoke token without confirmation
  construct daemon token revoke a1b2c3d4-5678-90ab-cdef-123456789012 --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			tokenID := args[0]

			if _, err := uuid.Parse(tokenID); err != nil {
				return fmt.Errorf("invalid token ID format: must be UUID")
			}

			if !options.Force {
				message := "This will permanently revoke the token.\nAny systems using this token will lose access immediately.\n\nAre you sure you want to continue?"
				if !confirm(cmd.InOrStdin(), cmd.OutOrStdout(), message) {
					return nil
				}
			}

			client := getAPIClient(cmd.Context())

			req := &connect.Request[v1.RevokeTokenRequest]{
				Msg: &v1.RevokeTokenRequest{Id: tokenID},
			}

			_, err := client.Auth().RevokeToken(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("failed to revoke token: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "✅ Token has been revoked")

			return nil
		},
	}

	cmd.Flags().BoolVarP(&options.Force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}
