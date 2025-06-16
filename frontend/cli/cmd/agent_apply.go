package cmd

import (
	"context"
	"fmt"
	"os"

	"connectrpc.com/connect"
	api "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type agentApplyOptions struct {
	Filename string
}

// AgentSpec represents the YAML structure for agent apply
type AgentSpec struct {
	ID           string `yaml:"id,omitempty"`
	Name         string `yaml:"name"`
	Description  string `yaml:"description,omitempty"`
	Instructions string `yaml:"instructions"`
	Model        string `yaml:"model"`
}

func NewAgentApplyCmd() *cobra.Command {
	var options agentApplyOptions

	cmd := &cobra.Command{
		Use:   "apply -f <file.yaml>",
		Short: "Apply agent configuration from a YAML file",
		Long: `Apply agent configuration from a YAML file.

This command reads an agent specification from a YAML file and either creates 
a new agent or updates an existing one based on whether an ID is specified 
in the file.

This is the declarative, automation-friendly way to manage agents, perfect 
for CI/CD pipelines and scripted workflows.`,
		Example: `  # Apply agent configuration from file
  construct agent apply -f coder.yaml

  # Get current config, modify, and apply
  construct agent get "coder" -o yaml > coder.yaml
  # ... edit coder.yaml ...
  construct agent apply -f coder.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if options.Filename == "" {
				return fmt.Errorf("filename is required. Use -f to specify the YAML file")
			}

			client := getAPIClient(cmd.Context())

			// Read and parse the YAML file
			spec, err := parseAgentSpecFile(options.Filename)
			if err != nil {
				return fmt.Errorf("failed to parse agent spec file: %w", err)
			}

			// Determine if this is a create or update operation
			if spec.ID == "" {
				// Create new agent
				return createAgentFromSpec(cmd.Context(), client, spec, cmd)
			} else {
				// Update existing agent
				return updateAgentFromSpec(cmd.Context(), client, spec, cmd)
			}
		},
	}

	cmd.Flags().StringVarP(&options.Filename, "filename", "f", "", "YAML file containing agent specification")
	cmd.MarkFlagRequired("filename")

	return cmd
}

func parseAgentSpecFile(filename string) (*AgentSpec, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var spec AgentSpec
	if err := yaml.Unmarshal(content, &spec); err != nil {
		return nil, fmt.Errorf("invalid YAML format: %w", err)
	}

	// Validate required fields
	if spec.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if spec.Instructions == "" {
		return nil, fmt.Errorf("instructions are required")
	}
	if spec.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	return &spec, nil
}

func createAgentFromSpec(ctx context.Context, client *api.Client, spec *AgentSpec, cmd *cobra.Command) error {
	// Resolve model ID if name was provided
	modelID := spec.Model
	if _, err := uuid.Parse(modelID); err != nil {
		// It's a model name, resolve to ID
		resolvedID, err := getModelID(ctx, client, modelID)
		if err != nil {
			return fmt.Errorf("failed to resolve model %s: %w", modelID, err)
		}
		modelID = resolvedID
	}

	// Create the agent
	agentResp, err := client.Agent().CreateAgent(ctx, &connect.Request[v1.CreateAgentRequest]{
		Msg: &v1.CreateAgentRequest{
			Name:         spec.Name,
			Description:  spec.Description,
			Instructions: spec.Instructions,
			ModelId:      modelID,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "agent.construct.ai/%s created\n", spec.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "ID: %s\n", agentResp.Msg.Agent.Id)
	return nil
}

func updateAgentFromSpec(ctx context.Context, client *api.Client, spec *AgentSpec, cmd *cobra.Command) error {
	// Validate that the ID is a valid UUID
	agentID := spec.ID
	if _, err := uuid.Parse(agentID); err != nil {
		return fmt.Errorf("invalid agent ID format: %s", agentID)
	}

	// Fetch current agent to see what changed
	currentAgentResp, err := client.Agent().GetAgent(ctx, &connect.Request[v1.GetAgentRequest]{
		Msg: &v1.GetAgentRequest{Id: agentID},
	})
	if err != nil {
		return fmt.Errorf("failed to get existing agent: %w", err)
	}

	currentAgent := currentAgentResp.Msg.Agent

	// Resolve model ID if name was provided
	modelID := spec.Model
	if _, err := uuid.Parse(modelID); err != nil {
		// It's a model name, resolve to ID
		resolvedID, err := getModelID(ctx, client, modelID)
		if err != nil {
			return fmt.Errorf("failed to resolve model %s: %w", modelID, err)
		}
		modelID = resolvedID
	}

	// Build update request with only changed fields
	updateReq := &v1.UpdateAgentRequest{
		Id: agentID,
	}

	// Check what fields have changed and set them in the update request
	if spec.Name != currentAgent.Metadata.Name {
		updateReq.Name = &spec.Name
	}
	if spec.Description != currentAgent.Metadata.Description {
		updateReq.Description = &spec.Description
	}
	if spec.Instructions != currentAgent.Spec.Instructions {
		updateReq.Instructions = &spec.Instructions
	}
	if modelID != currentAgent.Spec.ModelId {
		updateReq.ModelId = &modelID
	}

	// Apply the update
	_, err = client.Agent().UpdateAgent(ctx, &connect.Request[v1.UpdateAgentRequest]{
		Msg: updateReq,
	})
	if err != nil {
		return fmt.Errorf("failed to update agent: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "agent.construct.ai/%s configured\n", spec.Name)
	return nil
}
