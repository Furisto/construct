package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var modelProviderGetOptions struct {
	Id string
}

var modelProviderGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get a model provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getAPIClient(cmd.Context())

		resp, err := client.ModelProvider().GetModelProvider(cmd.Context(), &connect.Request[v1.GetModelProviderRequest]{
			Msg: &v1.GetModelProviderRequest{Id: modelProviderGetOptions.Id},
		})

		if err != nil {
			return err
		}

		return DisplayResources([]*ModelProviderDisplay{ConvertModelProviderToDisplay(resp.Msg.ModelProvider)}, formatOptions.Output)
	},
}

func init() {
	addFormatOptions(modelProviderGetCmd)
	modelProviderGetCmd.Flags().StringVarP(&modelProviderGetOptions.Id, "id", "i", "", "The ID of the model provider to get")
}
