package cmd

import (
	"connectrpc.com/connect"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var modelListOptions struct {
	ModelProviderID string
	Enabled         bool
	ShowDisabled    bool
}

var modelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List models",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getAPIClient(cmd.Context())

		filter := &v1.ListModelsRequest_Filter{}
		if modelListOptions.ModelProviderID != "" {
			filter.ModelProviderId = &modelListOptions.ModelProviderID
		}

		if !modelListOptions.ShowDisabled {
			enabled := true
			filter.Enabled = &enabled
		}

		req := &connect.Request[v1.ListModelsRequest]{
			Msg: &v1.ListModelsRequest{
				Filter: filter,
			},
		}

		resp, err := client.Model().ListModels(cmd.Context(), req)
		if err != nil {
			return err
		}

		displayModels := make([]*ModelDisplay, len(resp.Msg.Models))
		for i, model := range resp.Msg.Models {
			displayModels[i] = ConvertModelToDisplay(model)
		}

		return DisplayResources(displayModels, formatOptions.Output)
	},
}

func init() {
	addFormatOptions(modelListCmd)
	modelListCmd.Flags().StringVarP(&modelListOptions.ModelProviderID, "model-provider-id", "p", "", "Filter by model provider ID")
	modelListCmd.Flags().BoolVar(&modelListOptions.ShowDisabled, "show-disabled", false, "Show disabled models")
	modelCmd.AddCommand(modelListCmd)
}
