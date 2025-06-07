package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var agentGetOptions struct {
	Id            string
	FormatOptions FormatOptions
}

var agentGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get an agent by ID",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getAPIClient(cmd.Context())

		req := &connect.Request[v1.GetAgentRequest]{
			Msg: &v1.GetAgentRequest{Id: agentGetOptions.Id},
		}

		resp, err := client.Agent().GetAgent(cmd.Context(), req)
		if err != nil {
			return err
		}

		displayAgent := ConvertAgentToDisplay(resp.Msg.Agent)

		return getFormatter(cmd.Context()).Display([]*AgentDisplay{displayAgent}, agentGetOptions.FormatOptions.Output)
	},
}

func init() {
	addFormatOptions(agentGetCmd, &agentGetOptions.FormatOptions)
	agentCmd.AddCommand(agentGetCmd)
}
