package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

type modelGetOptions struct {
	RenderOptions RenderOptions
}

func NewModelGetCmd() *cobra.Command {
	var options modelGetOptions

	cmd := &cobra.Command{
		Use:   "get <model-id-or-name>",
		Short: "Get a model by ID or name",
		Long:  `Get detailed information about a model by specifying its ID or name.`,
		Example: `  # Get model by name
  construct model get "gpt-4"

  # Get model by ID
  construct model get "123e4567-e89b-12d3-a456-426614174000"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			modelID, err := getModelID(cmd.Context(), client, args[0])
			if err != nil {
				return fmt.Errorf("failed to resolve model %s: %w", args[0], err)
			}

			req := &connect.Request[v1.GetModelRequest]{
				Msg: &v1.GetModelRequest{Id: modelID},
			}

			resp, err := client.Model().GetModel(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("failed to get model %s: %w", args[0], err)
			}

			displayModel := ConvertModelToDisplay(resp.Msg.Model)
			return getRenderer(cmd.Context()).Render(displayModel, &options.RenderOptions)
		},
	}

	addRenderOptions(cmd, WithCardFormat(&options.RenderOptions))
	return cmd
}
