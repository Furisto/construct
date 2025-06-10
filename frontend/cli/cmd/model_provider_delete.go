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
		Use:   "delete <id-or-name>...",
		Short: "Delete one or more model providers by their IDs or names",
		Args:  cobra.MinimumNArgs(1),
		Aliases: []string{"rm"},
		Example: `  # Delete multiple model providers
  construct modelprovider delete anthropic-dev openai-prod

  # Delete model provider by ID
  construct modelprovider delete 01974c1d-0be8-70e1-88b4-ad9462fff25e`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			var modelProviderIDs = make(map[string]string)
			for _, idOrName := range args {
				modelProviderID, err := getModelProviderID(cmd.Context(), client, idOrName)
				if err != nil {
					return fmt.Errorf("failed to resolve model provider %s: %w", idOrName, err)
				}
				modelProviderIDs[idOrName] = modelProviderID
			}

			if !options.Force && !confirmDeletion(cmd.InOrStdin(), cmd.OutOrStdout(), "model-provider", args) {
				return nil
			}

			for idOrName, modelProviderID := range modelProviderIDs {
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
						Msg: &v1.DeleteModelRequest{Id: model.Id},
					})
					if err != nil {
						return fmt.Errorf("failed to delete model %s for model provider %s: %w", model.Name, idOrName, err)
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

	cmd.Flags().BoolVarP(&options.Force, "force", "f", false, "force deletion without confirmation")

	return cmd
}
