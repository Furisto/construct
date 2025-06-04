package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var agentDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete an agent by ID",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getAPIClient(cmd.Context())

		for _, id := range args {
			_, err := client.Agent().DeleteAgent(cmd.Context(), &connect.Request[v1.DeleteAgentRequest]{
				Msg: &v1.DeleteAgentRequest{Id: id},
			})
			if err != nil {
				return err
			}
		}

		return nil
	},
}

func init() {
	agentCmd.AddCommand(agentDeleteCmd)
}
