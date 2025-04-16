package cmd

import (
	"connectrpc.com/connect"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var modelProviderCreateOptions struct {
	Name   string
	ApiKey string
	Type   ModelProviderType
}

var modelProviderCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new model provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getClient()

		providerType, err := modelProviderCreateOptions.Type.ToAPI()
		if err != nil {
			return err
		}

		_, err = client.ModelProvider().CreateModelProvider(cmd.Context(), &connect.Request[v1.CreateModelProviderRequest]{
			Msg: &v1.CreateModelProviderRequest{
				Name:           modelProviderCreateOptions.Name,
				ProviderType:   providerType,
				Authentication: &v1.CreateModelProviderRequest_ApiKey{ApiKey: modelProviderCreateOptions.ApiKey},
			},
		})

		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	modelProviderCreateCmd.Flags().StringVarP(&modelProviderCreateOptions.Name, "name", "n", "", "The name of the model provider")
	modelProviderCreateCmd.Flags().StringVarP(&modelProviderCreateOptions.ApiKey, "api-key", "k", "", "The API key for the model provider")
	modelProviderCreateCmd.Flags().VarP(&modelProviderCreateOptions.Type, "type", "t", "The type of the model provider")

	modelProviderCmd.AddCommand(modelProviderCreateCmd)
}
