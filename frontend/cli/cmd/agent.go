package cmd

import (
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

func NewAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage agents",
		Long:  `Manage agents, including creation, deletion, retrieval, and listing.`,
	}

	cmd.AddCommand(NewAgentCreateCmd())
	cmd.AddCommand(NewAgentGetCmd())
	cmd.AddCommand(NewAgentListCmd())
	cmd.AddCommand(NewAgentDeleteCmd())

	return cmd
}

type AgentDisplay struct {
	ID           string   `json:"id" yaml:"id"`
	Name         string   `json:"name" yaml:"name"`
	Description  string   `json:"description,omitempty" yaml:"description,omitempty"`
	Instructions string   `json:"instructions" yaml:"instructions"`
	Model        string   `json:"model" yaml:"model"`
	DelegateIDs  []string `json:"delegateIds,omitempty" yaml:"delegateIds,omitempty"`
}

func ConvertAgentToDisplay(agent *v1.Agent) *AgentDisplay {
	if agent == nil || agent.Metadata == nil || agent.Spec == nil {
		return nil
	}
	return &AgentDisplay{
		ID:           agent.Id,
		Name:         agent.Metadata.Name,
		Description:  agent.Metadata.Description,
		Instructions: agent.Spec.Instructions,
		Model:        agent.Spec.ModelId,
		DelegateIDs:  agent.Spec.DelegateIds,
	}
}
