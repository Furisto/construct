package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

type modelProviderDeleteOptions struct {
	Force bool
}

func NewModelProviderDeleteCmd() *cobra.Command {
	options := new(modelProviderDeleteOptions)
	cmd := &cobra.Command{
		Use:     "delete <name|id>... [flags]",
		Short:   "Permanently delete one or more model providers",
		Args:    cobra.MinimumNArgs(1),
		Aliases: []string{"rm"},
		Long: `Permanently delete one or more model providers.

Deletes a provider configuration. **Warning**: This action will also delete all 
models that depend on this provider.`,
		Example: `  # Delete the 'anthropic-dev' provider
  construct provider delete anthropic-dev`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			type providerEntry struct {
				idOrName string
				id       string
			}
			var providers []providerEntry
			for _, idOrName := range args {
				modelProviderID, err := getModelProviderID(cmd.Context(), client, idOrName)
				if err != nil {
					return fmt.Errorf("failed to resolve model provider %s: %w", idOrName, err)
				}
				providers = append(providers, providerEntry{idOrName: idOrName, id: modelProviderID})
			}

			if !options.Force && !confirmDeletion(cmd.InOrStdin(), cmd.OutOrStdout(), "model-provider", args) {
				return nil
			}

			for _, provider := range providers {
				idOrName, modelProviderID := provider.idOrName, provider.id
				models, err := client.Model().ListModels(cmd.Context(), &connect.Request[v1.ListModelsRequest]{
					Msg: &v1.ListModelsRequest{
						Filter: &v1.ListModelsRequest_Filter{
							ModelProviderId: &modelProviderID,
						},
					},
				})
				if err != nil {
					return fmt.Errorf("failed to list models for model provider %s: %w", idOrName, err)
				}

				for _, model := range models.Msg.Models {
					_, err = client.Model().DeleteModel(cmd.Context(), &connect.Request[v1.DeleteModelRequest]{
						Msg: &v1.DeleteModelRequest{Id: model.Metadata.Id},
					})
					if err != nil {
						return fmt.Errorf("failed to delete model %s for model provider %s: %w", model.Spec.Name, idOrName, err)
					}
				}
				_, err = client.ModelProvider().DeleteModelProvider(cmd.Context(), &connect.Request[v1.DeleteModelProviderRequest]{
					Msg: &v1.DeleteModelProviderRequest{Id: modelProviderID},
				})
				if err != nil {
					return fmt.Errorf("failed to delete model provider %s: %w", idOrName, err)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&options.Force, "force", "f", false, "Skip the confirmation prompt")

	return cmd
}
