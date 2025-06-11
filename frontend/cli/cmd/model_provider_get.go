package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

type modelProviderGetOptions struct {
	RenderOptions RenderOptions
}

func NewModelProviderGetCmd() *cobra.Command {
	var options modelProviderGetOptions

	cmd := &cobra.Command{
		Use:   "get <id-or-name>",
		Short: "Get a model provider by ID or name",
		Args:  cobra.ExactArgs(1),
		Example: `  # Get model provider by name
  construct modelprovider get "anthropic-dev"

  # Get model provider with JSON output
  construct modelprovider get "openai-prod" --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())
			idOrName := args[0]

			modelProviderID, err := getModelProviderID(cmd.Context(), client, idOrName)
			if err != nil {
				return fmt.Errorf("failed to resolve model provider %s: %w", idOrName, err)
			}

			req := &connect.Request[v1.GetModelProviderRequest]{
				Msg: &v1.GetModelProviderRequest{Id: modelProviderID},
			}

			resp, err := client.ModelProvider().GetModelProvider(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("failed to get model provider %s: %w", idOrName, err)
			}

			displayModelProvider := ConvertModelProviderToDisplay(resp.Msg.ModelProvider)
			return getRenderer(cmd.Context()).Render(displayModelProvider, &options.RenderOptions)
		},
	}

	addRenderOptions(cmd, WithCardFormat(&options.RenderOptions))
	return cmd
}
