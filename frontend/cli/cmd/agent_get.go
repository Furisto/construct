package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

type agentGetOptions struct {
	RenderOptions RenderOptions
}

func NewAgentGetCmd() *cobra.Command {
	var options agentGetOptions

	cmd := &cobra.Command{
		Use:   "get <id-or-name>",
		Short: "Get an agent by ID or name",
		Args:  cobra.ExactArgs(1),
		Example: `  # Get agent by name
  construct agent get "coder"

  # Get agent by agent ID
  construct agent get 01974c1d-0be8-70e1-88b4-ad9462fff25e

  # Get agent with JSON output
  construct agent get "sql-expert" --output json

  # Get agent with YAML output
  construct agent get "reviewer" --output yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())
			idOrName := args[0]

			agentID, err := getAgentID(cmd.Context(), client, idOrName)
			if err != nil {
				return fmt.Errorf("failed to resolve agent %s: %w", idOrName, err)
			}

			req := &connect.Request[v1.GetAgentRequest]{
				Msg: &v1.GetAgentRequest{Id: agentID},
			}

			agentResp, err := client.Agent().GetAgent(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("failed to get agent %s: %w", idOrName, err)
			}

			modelResp, err := client.Model().GetModel(cmd.Context(), &connect.Request[v1.GetModelRequest]{
				Msg: &v1.GetModelRequest{
					Id: agentResp.Msg.Agent.Spec.ModelId,
				},
			})

			if err != nil {
				return fmt.Errorf("failed to get model %s: %w", agentResp.Msg.Agent.Spec.ModelId, err)
			}

			displayAgent := ConvertAgentToDisplay(agentResp.Msg.Agent, modelResp.Msg.Model)

			return getRenderer(cmd.Context()).Render(displayAgent, &options.RenderOptions)
		},
	}

	addRenderOptions(cmd, WithCardFormat(&options.RenderOptions))
	return cmd
}
