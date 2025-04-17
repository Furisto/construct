package cmd

import (
	"connectrpc.com/connect"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var agentCreateOptions struct {
	Name         string
	Description  string
	Instructions string
	ModelID      string
	DelegateIDs  []string
}

var agentCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getClient()

		_, err := client.Agent().CreateAgent(cmd.Context(), &connect.Request[v1.CreateAgentRequest]{
			Msg: &v1.CreateAgentRequest{
				Name:         agentCreateOptions.Name,
				Description:  agentCreateOptions.Description,
				Instructions: agentCreateOptions.Instructions,
				ModelId:      agentCreateOptions.ModelID,
				DelegateIds:  agentCreateOptions.DelegateIDs,
			},
		})

		if err != nil {
			return err
		}

		// TODO: Display the created agent? Similar to model get?

		return nil
	},
}

func init() {
	agentCreateCmd.Flags().StringVarP(&agentCreateOptions.Name, "name", "n", "", "The name of the agent")
	agentCreateCmd.Flags().StringVarP(&agentCreateOptions.Description, "description", "d", "", "The description of the agent (optional)")
	agentCreateCmd.Flags().StringVarP(&agentCreateOptions.Instructions, "instructions", "s", "", "The instructions for the agent")
	agentCreateCmd.Flags().StringVarP(&agentCreateOptions.ModelID, "model-id", "m", "", "The ID of the model to use")
	agentCreateCmd.Flags().StringSliceVarP(&agentCreateOptions.DelegateIDs, "delegate-ids", "e", []string{}, "The IDs of the delegate agents (optional)")

	agentCreateCmd.MarkFlagRequired("name")
	agentCreateCmd.MarkFlagRequired("instructions")
	agentCreateCmd.MarkFlagRequired("model-id")

	agentCmd.AddCommand(agentCreateCmd)
}
