package cmd

import (
	"connectrpc.com/connect"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var agentListOptions struct {
	ModelID       string
	FormatOptions FormatOptions
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List agents",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getAPIClient(cmd.Context())

		filter := &v1.ListAgentsRequest_Filter{}
		if agentListOptions.ModelID != "" {
			filter.ModelId = &agentListOptions.ModelID
		}

		req := &connect.Request[v1.ListAgentsRequest]{
			Msg: &v1.ListAgentsRequest{
				Filter: filter,
			},
		}

		resp, err := client.Agent().ListAgents(cmd.Context(), req)
		if err != nil {
			return err
		}

		displayAgents := make([]*AgentDisplay, len(resp.Msg.Agents))
		for i, agent := range resp.Msg.Agents {
			displayAgents[i] = ConvertAgentToDisplay(agent)
		}

		return getFormatter(cmd.Context()).Display(displayAgents, agentListOptions.FormatOptions.Output)
	},
}

func init() {
	agentListCmd.Flags().StringVarP(&agentListOptions.ModelID, "model", "m", "", "Filter agents by model ID")

	addFormatOptions(agentListCmd, &agentListOptions.FormatOptions)
	agentCmd.AddCommand(agentListCmd)
}
