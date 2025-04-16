package cmd

import (
	"connectrpc.com/connect"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var modelCreateOptions struct {
	Name            string
	ModelProviderID string
	ContextWindow   int64
}

var modelCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new model",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getClient()

		_, err := client.Model().CreateModel(cmd.Context(), &connect.Request[v1.CreateModelRequest]{
			Msg: &v1.CreateModelRequest{
				Name:            modelCreateOptions.Name,
				ModelProviderId: modelCreateOptions.ModelProviderID,
				ContextWindow:   modelCreateOptions.ContextWindow,
			},
		})

		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	modelCreateCmd.Flags().StringVarP(&modelCreateOptions.Name, "name", "n", "", "The name of the model")
	modelCreateCmd.Flags().StringVarP(&modelCreateOptions.ModelProviderID, "model-provider-id", "p", "", "The ID of the model provider")
	modelCreateCmd.Flags().Int64VarP(&modelCreateOptions.ContextWindow, "context-window", "w", 0, "The context window size")

	modelCmd.AddCommand(modelCreateCmd)
}
