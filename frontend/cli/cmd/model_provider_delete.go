package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var modelProviderDeleteOptions struct {
	Id string
}

var modelProviderDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a model provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getClient()

		_, err := client.ModelProvider().DeleteModelProvider(cmd.Context(), &connect.Request[v1.DeleteModelProviderRequest]{
			Msg: &v1.DeleteModelProviderRequest{Id: modelProviderDeleteOptions.Id},
		})

		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	modelProviderDeleteCmd.Flags().StringVarP(&modelProviderDeleteOptions.Id, "id", "i", "", "The ID of the model provider to delete")
}
