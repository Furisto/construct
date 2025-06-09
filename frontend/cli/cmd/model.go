package cmd

import (
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

func NewModelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "model",
		Short:   "Manage the AI models available to agents",
		GroupID: "resource",
	}

	cmd.AddCommand(NewModelCreateCmd())
	cmd.AddCommand(NewModelGetCmd())
	cmd.AddCommand(NewModelListCmd())
	cmd.AddCommand(NewModelDeleteCmd())

	return cmd
}

type ModelDisplay struct {
	Id              string   `json:"id"`
	Name            string   `json:"name"`
	ModelProviderID string   `json:"model_provider_id"`
	ContextWindow   int64    `json:"context_window"`
	Enabled         bool     `json:"enabled"`
	Capabilities    []string `json:"capabilities"`
}

func ConvertModelToDisplay(model *v1.Model) *ModelDisplay {
	capabilities := make([]string, len(model.Capabilities))
	for i, cap := range model.Capabilities {
		capabilities[i] = cap.String()
	}
	return &ModelDisplay{
		Id:              model.Id,
		Name:            model.Name,
		ModelProviderID: model.ModelProviderId,
		ContextWindow:   model.ContextWindow,
		Enabled:         model.Enabled,
		Capabilities:    capabilities,
	}
}
