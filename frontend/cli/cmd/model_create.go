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
		Use:   "create <name> [flags]",
		Short: "Register a new large language model for use by agents",
		Long: `Register a new large language model for use by agents.

Makes a specific model from a provider (like gpt-4o from OpenAI) available to 
construct. You must configure a provider before you can create a model.`,
		Example: `  # Register GPT-4o from the 'openai-prod' provider
  construct model create "gpt-4o" --provider "openai-prod" --context-window 128000

  # Register Claude Sonnet 3.5 from the 'anthropic' provider
  construct model create "claude-3-5-sonnet-20240620" --provider "anthropic" --context-window 200000`,
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
	cmd.Flags().Int64VarP(&options.ContextWindow, "context-window", "w", 0, "The maximum context window size for the model (required)")

	cmd.MarkFlagRequired("provider")
	cmd.MarkFlagRequired("context-window")

	return cmd
}
