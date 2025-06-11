package cmd

import (
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

func NewModelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "model",
		Short:   "Manage the AI models available to agents",
		Aliases: []string{"models"},
		GroupID: "resource",
	}

	cmd.AddCommand(NewModelCreateCmd())
	cmd.AddCommand(NewModelGetCmd())
	cmd.AddCommand(NewModelListCmd())
	cmd.AddCommand(NewModelDeleteCmd())

	return cmd
}

type ModelDisplay struct {
	Id            string   `json:"id" detail:"default"`
	Name          string   `json:"name" detail:"default"`
	ModelProvider string   `json:"model_provider" detail:"default"`
	ContextWindow int64    `json:"context_window" detail:"default"`
	Enabled       bool     `json:"enabled" detail:"full"`
	Capabilities  []string `json:"capabilities" detail:"full"`
}

func ConvertModelToDisplay(model *v1.Model) *ModelDisplay {
	capabilities := make([]string, len(model.Capabilities))
	for i, cap := range model.Capabilities {
		capabilities[i] = cap.String()
	}
	return &ModelDisplay{
		Id:            model.Id,
		Name:          model.Name,
		ModelProvider: model.ModelProviderId,
		ContextWindow: model.ContextWindow,
		Enabled:       model.Enabled,
		Capabilities:  capabilities,
	}
}
