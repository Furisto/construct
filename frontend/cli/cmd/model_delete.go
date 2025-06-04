package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var modelDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a model by ID",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getAPIClient(cmd.Context())

		for _, id := range args {
			_, err := client.Model().DeleteModel(cmd.Context(), &connect.Request[v1.DeleteModelRequest]{
				Msg: &v1.DeleteModelRequest{Id: id},
			})
			if err != nil {
				return err
			}
		}

		return nil
	},
}

func init() {
	modelCmd.AddCommand(modelDeleteCmd)
}
