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
			resp, err := client.ModelProvider().GetModelProvider(cmd.Context(), &connect.Request[v1.GetModelProviderRequest]{
				Msg: &v1.GetModelProviderRequest{Id: id},
			})

			if err != nil {
				return err
			}

			models, err := client.Model().ListModels(cmd.Context(), &connect.Request[v1.ListModelsRequest]{
				Msg: &v1.ListModelsRequest{
					Filter: &v1.ListModelsRequest_Filter{
						ModelProviderId: &resp.Msg.ModelProvider.Id,
					},
				},
			})

			if err != nil {
				return err
			}

			for _, model := range models.Msg.Models {
				_, err = client.Model().DeleteModel(cmd.Context(), &connect.Request[v1.DeleteModelRequest]{
					Msg: &v1.DeleteModelRequest{Id: model.Id},
				})

				if err != nil {
					return err
				}
			}
			
			_, err = client.ModelProvider().DeleteModelProvider(cmd.Context(), &connect.Request[v1.DeleteModelProviderRequest]{
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
