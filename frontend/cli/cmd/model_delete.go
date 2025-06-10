package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

type modelDeleteOptions struct {
	Force bool
}

func NewModelDeleteCmd() *cobra.Command {
	options := new(modelDeleteOptions)
	cmd := &cobra.Command{
		Use:   "delete <model-id-or-name>...",
		Short: "Delete one or more models by ID or name",
		Long:  `Delete models by specifying their IDs or names.`,
		Example: `  # Delete model by name
  construct model delete "gpt-4"

  # Delete multiple models
  construct model delete "claude-3-5-sonnet" "llama-3.1-8b" "gpt-4"`,
		Args:    cobra.MinimumNArgs(1),
		Aliases: []string{"rm"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			modelIDs := make([]string, len(args))
			for i, modelNameOrID := range args {
				modelID, err := getModelID(cmd.Context(), client, modelNameOrID)
				if err != nil {
					return fmt.Errorf("failed to resolve model %s: %w", modelNameOrID, err)
				}
				modelIDs[i] = modelID
			}

			if !options.Force && !confirmDeletion(cmd.InOrStdin(), cmd.OutOrStdout(), "model", args) {
				return nil
			}

			for i, modelID := range modelIDs {
				_, err := client.Model().DeleteModel(cmd.Context(), &connect.Request[v1.DeleteModelRequest]{
					Msg: &v1.DeleteModelRequest{Id: modelID},
				})

				if err != nil {
					return fmt.Errorf("failed to delete model %s: %w", args[i], err)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&options.Force, "force", "f", false, "force deletion without confirmation")

	return cmd
}
