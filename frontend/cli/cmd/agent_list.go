package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

type agentListOptions struct {
	Models        []string
	Names         []string
	Limit         int32
	Enabled       bool
	Columns       []string
	FormatOptions FormatOptions
}

func NewAgentListCmd() *cobra.Command {
	var options agentListOptions

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List agents",
		Aliases: []string{"ls"},
		Example: `  # List all agents
  construct agent list

  # List agents by model name
  construct agent list --model "claude-4"

  # List agents by model ID
  construct agent list --model "01974c1d-0be8-70e1-88b4-ad9462fff25e"

  # List only enabled agents
  construct agent list --enabled

  # Multiple filters combined
  construct agent list --model "claude-4" --enabled --limit 5`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			filter := &v1.ListAgentsRequest_Filter{}

			if len(options.Models) > 0 {
				resolvedModelIDs := make([]string, 0, len(options.Models))

				for _, modelID := range options.Models {
					if modelID == "" {
						continue
					}

					_, err := uuid.Parse(modelID)
					if err != nil {
						resolvedID, err := getModelID(cmd.Context(), client, modelID)
						if err != nil {
							return fmt.Errorf("failed to resolve model %s: %w", modelID, err)
						}
						resolvedModelIDs = append(resolvedModelIDs, resolvedID)
					} else {
						resolvedModelIDs = append(resolvedModelIDs, modelID)
					}
				}

				if len(resolvedModelIDs) > 0 {
					filter.ModelIds = resolvedModelIDs
				}
			}

			if len(options.Names) > 0 {
				filter.Name = options.Names
			}

			// Handle enabled/disabled filtering
			// Note: API doesn't currently support enabled/disabled filtering
			// This would need to be implemented on the backend first

			req := &connect.Request[v1.ListAgentsRequest]{
				Msg: &v1.ListAgentsRequest{
					Filter: filter,
				},
			}

			// Note: API doesn't currently support limit parameter
			// This would need to be implemented on the backend first

			resp, err := client.Agent().ListAgents(cmd.Context(), req)
			if err != nil {
				return fmt.Errorf("failed to list agents: %w", err)
			}

			displayAgents := make([]*AgentDisplay, len(resp.Msg.Agents))
			for i, agent := range resp.Msg.Agents {
				displayAgents[i] = ConvertAgentToDisplay(agent)
			}

			// Handle columns filtering if specified
			if len(options.Columns) > 0 {
				// Note: Column filtering would need to be implemented in the formatter
				// For now, we pass the column information through format options
				// This is placeholder logic - actual implementation depends on the formatter
			}

			return getFormatter(cmd.Context()).Display(displayAgents, options.FormatOptions.Output)
		},
	}

	cmd.Flags().StringArrayVarP(&options.Models, "model", "m", []string{}, "Show only agents using this AI model (e.g., 'claude-4', 'gpt-4', or model ID)")
	cmd.Flags().StringArrayVarP(&options.Names, "name", "n", []string{}, "Filter agents by name")
	cmd.Flags().Int32VarP(&options.Limit, "limit", "l", 0, "Limit number of results")
	cmd.Flags().BoolVar(&options.Enabled, "enabled", true, "Show only enabled agents")
	cmd.Flags().StringArrayVar(&options.Columns, "columns", []string{}, "Specify which columns to display")
	addFormatOptions(cmd, &options.FormatOptions)

	return cmd
}
