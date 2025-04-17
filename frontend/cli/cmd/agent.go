package cmd

import (
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage agents",
	Long:  `Manage agents, including creation, deletion, retrieval, and listing.`,
}

func init() {
	rootCmd.AddCommand(agentCmd)
}

type AgentDisplay struct {
	ID           string   `json:"id" yaml:"id"`
	Name         string   `json:"name" yaml:"name"`
	Description  string   `json:"description,omitempty" yaml:"description,omitempty"`
	Instructions string   `json:"instructions" yaml:"instructions"`
	ModelID      string   `json:"modelId" yaml:"modelId"`
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
		ModelID:      agent.Spec.ModelId,
		DelegateIDs:  agent.Spec.DelegateIds,
	}
}
