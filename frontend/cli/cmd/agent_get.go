package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var agentGetOptions struct {
	Id string
}

var agentGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get an agent by ID",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getAPIClient()

		req := &connect.Request[v1.GetAgentRequest]{
			Msg: &v1.GetAgentRequest{Id: agentGetOptions.Id},
		}

		resp, err := client.Agent().GetAgent(cmd.Context(), req)
		if err != nil {
			return err
		}

		displayAgent := ConvertAgentToDisplay(resp.Msg.Agent)

		return DisplayResources([]*AgentDisplay{displayAgent}, formatOptions.Output)
	},
}

func init() {
	addFormatOptions(agentGetCmd)
	agentGetCmd.Flags().StringVarP(&agentGetOptions.Id, "id", "i", "", "The ID of the agent to get")
	agentGetCmd.MarkFlagRequired("id")

	agentCmd.AddCommand(agentGetCmd)
}
