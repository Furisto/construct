package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var modelDeleteOptions struct {
	Id string
}

var modelDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a model by ID",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getClient()

		req := &connect.Request[v1.DeleteModelRequest]{
			Msg: &v1.DeleteModelRequest{Id: modelDeleteOptions.Id},
		}

		_, err := client.Model().DeleteModel(cmd.Context(), req)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	modelDeleteCmd.Flags().StringVarP(&modelDeleteOptions.Id, "id", "i", "", "The ID of the model to delete")
	modelDeleteCmd.MarkFlagRequired("id")
	modelCmd.AddCommand(modelDeleteCmd)
}
