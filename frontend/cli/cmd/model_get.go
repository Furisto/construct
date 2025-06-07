package cmd

import (
	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var modelGetOptions struct {
	Id            string
	FormatOptions FormatOptions
}

var modelGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get a model by ID",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getAPIClient(cmd.Context())

		req := &connect.Request[v1.GetModelRequest]{
			Msg: &v1.GetModelRequest{Id: modelGetOptions.Id},
		}

		resp, err := client.Model().GetModel(cmd.Context(), req)
		if err != nil {
			return err
		}

		displayModel := ConvertModelToDisplay(resp.Msg.Model)

		return getFormatter(cmd.Context()).Display([]*ModelDisplay{displayModel}, modelGetOptions.FormatOptions.Output)
	},
}

func init() {
	addFormatOptions(modelGetCmd, &modelGetOptions.FormatOptions)
	modelGetCmd.Flags().StringVarP(&modelGetOptions.Id, "id", "i", "", "The ID of the model to get")
	modelGetCmd.MarkFlagRequired("id")
	modelCmd.AddCommand(modelGetCmd)
}
