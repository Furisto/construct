package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var messageDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a message",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getAPIClient(cmd.Context())

		_, err := client.Message().DeleteMessage(cmd.Context(), &connect.Request[v1.DeleteMessageRequest]{
			Msg: &v1.DeleteMessageRequest{Id: args[0]},
		})

		return err
	},
}

func init() {
	messageCmd.AddCommand(messageDeleteCmd)
}
