package cmd

import (
	"connectrpc.com/connect"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var agentCreateOptions struct {
	Name         string
	Description  string
	SystemPrompt string
	ModelID      string
}

var agentCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getAPIClient(cmd.Context())

		_, err := client.Agent().CreateAgent(cmd.Context(), &connect.Request[v1.CreateAgentRequest]{
			Msg: &v1.CreateAgentRequest{
				Name:         agentCreateOptions.Name,
				Description:  agentCreateOptions.Description,
				Instructions: agentCreateOptions.SystemPrompt,
				ModelId:      agentCreateOptions.ModelID,
			},
		})

		return err
	},
}

func init() {
	agentCreateCmd.Flags().StringVarP(&agentCreateOptions.Name, "name", "n", "", "The name of the agent")
	agentCreateCmd.Flags().StringVarP(&agentCreateOptions.Description, "description", "d", "", "The description of the agent (optional)")
	agentCreateCmd.Flags().StringVarP(&agentCreateOptions.SystemPrompt, "prompt", "p", "", "The prompt for the agent")
	agentCreateCmd.Flags().StringVarP(&agentCreateOptions.ModelID, "model", "m", "", "The model to use")

	agentCreateCmd.MarkFlagRequired("name")
	agentCreateCmd.MarkFlagRequired("prompt")
	agentCreateCmd.MarkFlagRequired("model")

	agentCmd.AddCommand(agentCreateCmd)
}
