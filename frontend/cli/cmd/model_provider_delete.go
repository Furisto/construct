package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var modelProviderDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a model provider",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getClient()

		for _, id := range args {
			_, err := client.ModelProvider().DeleteModelProvider(cmd.Context(), &connect.Request[v1.DeleteModelProviderRequest]{
				Msg: &v1.DeleteModelProviderRequest{Id: id},
			})

			if err != nil {
				return err
			}
		}

		return nil
	},
}

func init() {
	modelProviderCmd.AddCommand(modelProviderDeleteCmd)
}
