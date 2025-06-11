package cmd

import (
	"fmt"

	"connectrpc.com/connect"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

type modelListOptions struct {
	ModelProvider string
	ShowDisabled  bool
	RenderOptions RenderOptions
}

func NewModelListCmd() *cobra.Command {
	var options modelListOptions

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List models",
		Aliases: []string{"ls"},
		Long:    `List models.`,
		Example: `  # List all models
  construct model list

  # List models by provider name
  construct model list --model-provider "anthropic-dev"

  # List all models including disabled ones
  construct model list --show-disabled`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			filter := &v1.ListModelsRequest_Filter{}

			if options.ModelProvider != "" {
				modelProviderID, err := getModelProviderID(cmd.Context(), client, options.ModelProvider)
				if err != nil {
					return fmt.Errorf("failed to resolve model provider %s: %w", options.ModelProvider, err)
				}
				filter.ModelProviderId = &modelProviderID
			}

			if !options.ShowDisabled {
				enabled := true
				filter.Enabled = &enabled
			}

			req := &connect.Request[v1.ListModelsRequest]{
				Msg: &v1.ListModelsRequest{
					Filter: filter,
				},
			}

			resp, err := client.Model().ListModels(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("failed to list models: %w", err)
			}

			displayModels := make([]*ModelDisplay, len(resp.Msg.Models))
			for i, model := range resp.Msg.Models {
				displayModels[i] = ConvertModelToDisplay(model)
			}

			return getRenderer(cmd.Context()).Render(displayModels, &options.RenderOptions)
		},
	}

	cmd.Flags().StringVarP(&options.ModelProvider, "model-provider", "p", "", "Filter by model provider name or ID")
	cmd.Flags().BoolVarP(&options.ShowDisabled, "show-disabled", "d", false, "Show disabled models")
	addRenderOptions(cmd, &options.RenderOptions)
	return cmd
}
