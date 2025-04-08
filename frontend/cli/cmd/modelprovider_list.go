package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var modelProviderListCmd = &cobra.Command{
	Use:   "list",
	Short: "List model providers",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getClient()

		resp, err := client.ModelProvider().ListModelProviders(cmd.Context(), &connect.Request[v1.ListModelProvidersRequest]{})
		if err != nil {
			return err
		}

		modelProviders := make([]*ModelProviderDisplay, len(resp.Msg.ModelProviders))
		for i, modelProvider := range resp.Msg.ModelProviders {
			modelProviders[i] = ConvertModelProviderToDisplay(modelProvider)
		}

		return DisplayResources(modelProviders, formatOptions.Output)
	},
}

func init() {
	addFormatOptions(modelProviderListCmd)
	modelProviderCmd.AddCommand(modelProviderListCmd)
}
