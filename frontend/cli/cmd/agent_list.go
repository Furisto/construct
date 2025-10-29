package cmd

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	api "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/frontend/cli/pkg/fail"
	"github.com/spf13/cobra"
)

type agentListOptions struct {
	Models        []string
	Names         []string
	Limit         int32
	RenderOptions RenderOptions
}

func NewAgentListCmd() *cobra.Command {
	var options agentListOptions

	cmd := &cobra.Command{
		Use:     "list [flags]",
		Short:   "List all available agents",
		Aliases: []string{"ls"},
		Example: `  # List all agents in a table
  construct agent list

  # Find all agents using a specific model
  construct agent ls --model "claude-3-5-sonnet"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			agents, err := agentList(cmd.Context(), options, client)
			if err != nil {
				return fail.HandleError(cmd, err)
			}

			return getRenderer(cmd.Context()).Render(agents, &options.RenderOptions)
		},
	}

	cmd.Flags().StringArrayVarP(&options.Models, "model", "m", []string{}, "Filter agents by the model they use")
	cmd.Flags().StringArrayVarP(&options.Names, "name", "n", []string{}, "Filter agents by name (supports partial matching)")
	cmd.Flags().Int32VarP(&options.Limit, "limit", "l", 0, "Limit the number of results returned")
	addRenderOptions(cmd, &options.RenderOptions)

	return cmd
}

func agentList(ctx context.Context, options agentListOptions, client *api.Client) ([]*AgentDisplay, error) {
	filter := &v1.ListAgentsRequest_Filter{}

	if len(options.Names) > 0 {
		filter.Names = options.Names
	}

	// Resolve model names to IDs for client-side filtering
	var modelIDs []string
	if len(options.Models) > 0 {
		for _, model := range options.Models {
			modelID, err := getModelID(ctx, client, model)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve model %s: %w", model, err)
			}
			modelIDs = append(modelIDs, modelID)
		}
	}

	req := &connect.Request[v1.ListAgentsRequest]{
		Msg: &v1.ListAgentsRequest{
			Filter: filter,
		},
	}

	resp, err := client.Agent().ListAgents(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}

	// Client-side filtering by model IDs
	filteredAgents := resp.Msg.Agents
	if len(modelIDs) > 0 {
		filteredAgents = make([]*v1.Agent, 0)
		modelIDSet := make(map[string]bool)
		for _, id := range modelIDs {
			modelIDSet[id] = true
		}
		for _, agent := range resp.Msg.Agents {
			if modelIDSet[agent.Spec.ModelId] {
				filteredAgents = append(filteredAgents, agent)
			}
		}
	}

	displayAgents := make([]*AgentDisplay, len(filteredAgents))
	for i, agent := range filteredAgents {
		model, err := client.Model().GetModel(ctx, &connect.Request[v1.GetModelRequest]{
			Msg: &v1.GetModelRequest{
				Id: agent.Spec.ModelId,
			},
		})

		if err != nil {
			return nil, fmt.Errorf("failed to get model %s: %w", agent.Spec.ModelId, err)
		}

		displayAgents[i] = ConvertAgentToDisplay(agent, model.Msg.Model.Spec.Name)
	}

	return displayAgents, nil
}
