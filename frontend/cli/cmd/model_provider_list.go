package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

type modelProviderListOptions struct {
	ProviderTypes ModelProviderTypes
	Enabled       bool
	RenderOptions RenderOptions
}

func NewModelProviderListCmd() *cobra.Command {
	var options modelProviderListOptions

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List model providers",
		Aliases: []string{"ls"},
		Example: `  # List all model providers including disabled ones
  construct modelprovider list --enabled=false

  # List multiple provider types
  construct modelprovider list --provider-type anthropic --provider-type openai`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			filter := &v1.ListModelProvidersRequest_Filter{}

			if len(options.ProviderTypes) > 0 {
				providerTypes := make([]v1.ModelProviderType, 0, len(options.ProviderTypes))
				for _, providerType := range options.ProviderTypes {
					apiProviderType, err := providerType.ToAPI()
					if err != nil {
						return fmt.Errorf("failed to convert provider type '%s': %w", providerType, err)
					}
					providerTypes = append(providerTypes, apiProviderType)
				}
				filter.ProviderTypes = providerTypes
			}

			req := &connect.Request[v1.ListModelProvidersRequest]{
				Msg: &v1.ListModelProvidersRequest{
					Filter: filter,
				},
			}

			resp, err := client.ModelProvider().ListModelProviders(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("failed to list model providers: %w", err)
			}

			displayModelProviders := make([]*ModelProviderDisplay, len(resp.Msg.ModelProviders))
			for i, modelProvider := range resp.Msg.ModelProviders {
				displayModelProviders[i] = ConvertModelProviderToDisplay(modelProvider)
			}

			return getRenderer(cmd.Context()).Render(displayModelProviders, &options.RenderOptions)
		},
	}

	cmd.Flags().VarP(&options.ProviderTypes, "provider-type", "t", "Filter by provider type (anthropic, openai)")
	cmd.Flags().BoolVar(&options.Enabled, "enabled", true, "Show only enabled model providers")
	addRenderOptions(cmd, &options.RenderOptions)
	return cmd
}
