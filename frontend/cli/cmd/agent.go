package cmd

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	api "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func NewAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "agent",
		Short:   "Create, list, and configure reusable agents",
		Aliases: []string{"agents"},
		GroupID: "resource",
	}

	cmd.AddCommand(NewAgentCreateCmd())
	cmd.AddCommand(NewAgentGetCmd())
	cmd.AddCommand(NewAgentListCmd())
	cmd.AddCommand(NewAgentDeleteCmd())

	return cmd
}

type AgentDisplay struct {
	ID           string `json:"id" yaml:"id" detail:"default"`
	Name         string `json:"name" yaml:"name" detail:"default"`
	Description  string `json:"description,omitempty" yaml:"description,omitempty" detail:"default"`
	Instructions string `json:"instructions" yaml:"instructions"`
	Model        string `json:"model" yaml:"model" detail:"default"`
	CreatedAt    string `json:"created_at" yaml:"created_at" detail:"full"`
}

func ConvertAgentToDisplay(agent *v1.Agent, model *v1.Model) *AgentDisplay {
	if agent == nil || agent.Metadata == nil || agent.Spec == nil {
		return nil
	}
	return &AgentDisplay{
		ID:           agent.Id,
		Name:         agent.Metadata.Name,
		Description:  agent.Metadata.Description,
		Instructions: agent.Spec.Instructions,
		Model:        model.Name,
	}
}

func getAgentID(ctx context.Context, client *api.Client, idOrName string) (string, error) {
	_, err := uuid.Parse(idOrName)
	if err == nil {
		return idOrName, nil
	}

	agentResp, err := client.Agent().ListAgents(ctx, &connect.Request[v1.ListAgentsRequest]{
		Msg: &v1.ListAgentsRequest{
			Filter: &v1.ListAgentsRequest_Filter{
				Name: []string{idOrName},
			},
		},
	})

	if err != nil {
		return "", fmt.Errorf("failed to list agents: %w", err)
	}

	if len(agentResp.Msg.Agents) == 0 {
		return "", fmt.Errorf("agent %s not found", idOrName)
	}

	if len(agentResp.Msg.Agents) > 1 {
		return "", fmt.Errorf("multiple agents found for %s", idOrName)
	}

	return agentResp.Msg.Agents[0].Id, nil
}

func getModelID(ctx context.Context, client *api.Client, idOrName string) (string, error) {
	_, err := uuid.Parse(idOrName)
	if err == nil {
		return idOrName, nil
	}

	// todo: consider using fuzzy matching
	modelResp, err := client.Model().ListModels(ctx, &connect.Request[v1.ListModelsRequest]{
		Msg: &v1.ListModelsRequest{
			Filter: &v1.ListModelsRequest_Filter{
				Name: api.Ptr(idOrName),
			},
		},
	})

	if err != nil {
		return "", fmt.Errorf("failed to list models: %w", err)
	}

	if len(modelResp.Msg.Models) == 0 {
		return "", fmt.Errorf("model %s not found", idOrName)
	}

	if len(modelResp.Msg.Models) > 1 {
		return "", fmt.Errorf("multiple models found for %s", idOrName)
	}

	return modelResp.Msg.Models[0].Id, nil
}
