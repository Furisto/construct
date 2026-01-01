package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

type tokenListOptions struct {
	NamePrefix     string
	IncludeExpired bool
	RenderOptions  RenderOptions
}

func NewDaemonTokenListCmd() *cobra.Command {
	var options tokenListOptions

	cmd := &cobra.Command{
		Use:     "list [flags]",
		Short:   "List all tokens with metadata",
		Aliases: []string{"ls"},
		Long: `List all tokens with metadata.

Displays token ID, name, creation time, expiration time, and status. Token
values are never shown - only metadata. Use filters to narrow results.`,
		Example: `  # List all active tokens
  construct daemon token list

  # Filter by name prefix
  construct daemon token list --name-prefix prod

  # Include expired tokens
  construct daemon token list --include-expired

  # JSON output for scripting
  construct daemon token ls --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			req := &connect.Request[v1.ListTokensRequest]{
				Msg: &v1.ListTokensRequest{
					IncludeExpired: options.IncludeExpired,
				},
			}

			if options.NamePrefix != "" {
				req.Msg.NamePrefix = options.NamePrefix
			}

			resp, err := client.Auth().ListTokens(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("failed to list tokens: %w", err)
			}

			displayTokens := make([]*TokenDisplay, len(resp.Msg.Tokens))
			for i, token := range resp.Msg.Tokens {
				displayTokens[i] = ConvertTokenInfoToDisplay(token)
			}

			return getRenderer(cmd.Context()).Render(displayTokens, &options.RenderOptions)
		},
	}

	cmd.Flags().StringVar(&options.NamePrefix, "name-prefix", "", "Filter by name prefix")
	cmd.Flags().BoolVar(&options.IncludeExpired, "include-expired", false, "Include expired tokens in results")
	addRenderOptions(cmd, &options.RenderOptions)

	return cmd
}
