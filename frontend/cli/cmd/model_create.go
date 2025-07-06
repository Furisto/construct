package cmd

import (
	"fmt"

	"connectrpc.com/connect"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

type modelCreateOptions struct {
	ModelProvider string
	ContextWindow int64
}

func NewModelCreateCmd() *cobra.Command {
	var options modelCreateOptions

	cmd := &cobra.Command{
		Use:   "create <model-name> --provider <provider-name> --context-window <context-window>",
		Short: "Create a new model",
		Long:  `Create a new model with the specified name, model provider, and context window.`,
		Example: `  # Create a model with a specific provider name
  construct model create "gpt-4" --provider "openai-dev" --context-window 8192

  # Create a model using provider ID
  construct model create "claude-3-5-sonnet" --provider "123e4567-e89b-12d3-a456-426614174000" --context-window 200000`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			client := getAPIClient(cmd.Context())

			modelProviderID, err := getModelProviderID(cmd.Context(), client, options.ModelProvider)
			if err != nil {
				return fmt.Errorf("failed to resolve model provider %s: %w", options.ModelProvider, err)
			}

			resp, err := client.Model().CreateModel(cmd.Context(), &connect.Request[v1.CreateModelRequest]{
				Msg: &v1.CreateModelRequest{
					Name:            name,
					ModelProviderId: modelProviderID,
					ContextWindow:   options.ContextWindow,
				},
			})

			if err != nil {
				return fmt.Errorf("failed to create model: %w", err)
			}

			cmd.Println(resp.Msg.Model.Metadata.Id)
			return nil
		},
	}

	cmd.Flags().StringVarP(&options.ModelProvider, "provider", "p", "", "The name or ID of the model provider (required)")
	cmd.Flags().Int64VarP(&options.ContextWindow, "context-window", "w", 0, "The context window size (required)")

	cmd.MarkFlagRequired("model-provider")
	cmd.MarkFlagRequired("context-window")

	return cmd
}
