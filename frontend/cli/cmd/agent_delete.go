package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

func NewAgentDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id-or-name>...",
		Short: "Delete one or more agents by their IDs or names",
		Example: `  # Delete multiple agents
  construct agent delete coder architect debugger

  # Delete agent by agent ID
  construct agent delete 01974c1d-0be8-70e1-88b4-ad9462fff25e`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())

			var agentIDs = make(map[string]string)
			for _, idOrName := range args {
				agentID, err := getAgentID(cmd.Context(), client, idOrName)
				if err != nil {
					return err
				}

				agentIDs[idOrName] = agentID
			}

			for idOrName, agentID := range agentIDs {
				_, err := client.Agent().DeleteAgent(cmd.Context(), &connect.Request[v1.DeleteAgentRequest]{
					Msg: &v1.DeleteAgentRequest{Id: agentID},
				})
				if err != nil {
					return fmt.Errorf("failed to delete agent %s: %w", idOrName, err)
				}
			}

			return nil
		},
	}

	return cmd
}
