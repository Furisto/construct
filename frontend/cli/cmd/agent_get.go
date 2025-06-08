package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

type agentGetOptions struct {
	Id            string
	FormatOptions FormatOptions
}

func NewAgentGetCmd() *cobra.Command {
	var options agentGetOptions

	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get an agent by ID",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			req := &connect.Request[v1.GetAgentRequest]{
				Msg: &v1.GetAgentRequest{Id: options.Id},
			}

			resp, err := client.Agent().GetAgent(cmd.Context(), req)
			if err != nil {
				return err
			}

			displayAgent := ConvertAgentToDisplay(resp.Msg.Agent)

			return getFormatter(cmd.Context()).Display([]*AgentDisplay{displayAgent}, options.FormatOptions.Output)
		},
	}

	addFormatOptions(cmd, &options.FormatOptions)
	return cmd
}
