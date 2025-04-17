package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var agentDeleteOptions struct {
	Id string
}

var agentDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete an agent by ID",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getClient()

		req := &connect.Request[v1.DeleteAgentRequest]{
			Msg: &v1.DeleteAgentRequest{Id: agentDeleteOptions.Id},
		}

		_, err := client.Agent().DeleteAgent(cmd.Context(), req)
		if err != nil {
			return err
		}
		
		return nil
	},
}

func init() {
	agentDeleteCmd.Flags().StringVarP(&agentDeleteOptions.Id, "id", "i", "", "The ID of the agent to delete")
	agentDeleteCmd.MarkFlagRequired("id")
	agentCmd.AddCommand(agentDeleteCmd)
}
