package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"

	"connectrpc.com/connect"
	api "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// AgentEditSpec represents the editable structure of an agent
type AgentEditSpec struct {
	// Commented header to guide users
	ID           string `yaml:"id" comment:"# Agent ID (read-only)"`
	Name         string `yaml:"name" comment:"# Agent name"`
	Description  string `yaml:"description,omitempty" comment:"# Agent description (optional)"`
	Instructions string `yaml:"instructions" comment:"# System instructions/prompt"`
	Model        string `yaml:"model" comment:"# Model name or ID"`
}

func NewAgentEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <id-or-name>",
		Short: "Edit an agent configuration interactively using your default editor",
		Long: `Edit an agent configuration using your default editor ($EDITOR).

This command fetches the current agent configuration, opens it as a YAML file 
in your terminal editor, and applies any changes you make upon saving and 
closing the editor.

Similar to 'kubectl edit', this provides a powerful way to make multiple 
changes to an agent in a single operation.`,
		Args: cobra.ExactArgs(1),
		Example: `  # Edit agent by name
  construct agent edit "coder"

  # Edit agent by ID  
  construct agent edit 01974c1d-0be8-70e1-88b4-ad9462fff25e

  # Set custom editor
  EDITOR=nano construct agent edit "coder"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getAPIClient(cmd.Context())
			idOrName := args[0]

			// Resolve agent ID
			agentID, err := getAgentID(cmd.Context(), client, idOrName)
			if err != nil {
				return fmt.Errorf("failed to resolve agent %s: %w", idOrName, err)
			}

			// Fetch current agent configuration
			agentResp, err := client.Agent().GetAgent(cmd.Context(), &connect.Request[v1.GetAgentRequest]{
				Msg: &v1.GetAgentRequest{Id: agentID},
			})
			if err != nil {
				return fmt.Errorf("failed to get agent %s: %w", idOrName, err)
			}

			// Fetch model information for display
			modelResp, err := client.Model().GetModel(cmd.Context(), &connect.Request[v1.GetModelRequest]{
				Msg: &v1.GetModelRequest{
					Id: agentResp.Msg.Agent.Spec.ModelId,
				},
			})
			if err != nil {
				return fmt.Errorf("failed to get model %s: %w", agentResp.Msg.Agent.Spec.ModelId, err)
			}

			// Convert to editable spec
			editSpec := &AgentEditSpec{
				ID:           agentResp.Msg.Agent.Id,
				Name:         agentResp.Msg.Agent.Metadata.Name,
				Description:  agentResp.Msg.Agent.Metadata.Description,
				Instructions: agentResp.Msg.Agent.Spec.Instructions,
				Model:        modelResp.Msg.Model.Name,
			}

			// Save original spec for comparison
			originalSpec := *editSpec

			// Create temporary file with YAML content
			tempFile, err := createTempYAMLFile(editSpec)
			if err != nil {
				return fmt.Errorf("failed to create temporary file: %w", err)
			}
			defer os.Remove(tempFile)

			// Open editor
			if err := openEditor(tempFile); err != nil {
				return fmt.Errorf("failed to open editor: %w", err)
			}

			// Parse edited content
			editedSpec, err := parseYAMLFile(tempFile)
			if err != nil {
				return fmt.Errorf("failed to parse edited content: %w", err)
			}

			// Check if any changes were made
			if reflect.DeepEqual(originalSpec, *editedSpec) {
				fmt.Fprintln(cmd.OutOrStdout(), "No changes made.")
				return nil
			}

			// Validate that ID hasn't changed
			if editedSpec.ID != originalSpec.ID {
				return fmt.Errorf("agent ID cannot be modified")
			}

			// Apply the changes
			if err := applyAgentChanges(cmd.Context(), client, agentResp.Msg.Agent, editedSpec); err != nil {
				return fmt.Errorf("failed to apply changes: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "agent.construct.ai/%s edited\n", editedSpec.Name)
			return nil
		},
	}

	return cmd
}

func createTempYAMLFile(spec *AgentEditSpec) (string, error) {
	// Create temporary file
	tempFile, err := os.CreateTemp("", "construct-agent-*.yaml")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	// Create YAML content with helpful comments
	content := fmt.Sprintf(`# Please edit the object below. Lines beginning with a '#' will be ignored,
# and an empty file will abort the edit. If an error occurs while saving this file will be
# reopened with the relevant failures.
#
id: %s
name: %s
description: %s
instructions: |
%s
model: %s
`,
		spec.ID,
		spec.Name,
		spec.Description,
		indentString(spec.Instructions, "  "),
		spec.Model,
	)

	if _, err := tempFile.WriteString(content); err != nil {
		return "", err
	}

	return tempFile.Name(), nil
}

func openEditor(filename string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editors := []string{"code", "cursor", "vim", "nano", "emacs", "vi"}
		for _, e := range editors {
			if _, err := exec.LookPath(e); err == nil {
				editor = e
				break
			}
		}
	}

	if editor == "" {
		return fmt.Errorf("no editor found. Please set the EDITOR environment variable")
	}

	cmd := exec.Command(editor, filename)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func parseYAMLFile(filename string) (*AgentEditSpec, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var spec AgentEditSpec
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

func applyAgentChanges(ctx context.Context, client *api.Client, currentAgent *v1.Agent, editedSpec *AgentEditSpec) error {
	// Resolve model ID if name was provided
	modelID := editedSpec.Model
	if _, err := uuid.Parse(modelID); err != nil {
		// It's a model name, resolve to ID
		resolvedID, err := getModelID(ctx, client, modelID)
		if err != nil {
			return fmt.Errorf("failed to resolve model %s: %w", modelID, err)
		}
		modelID = resolvedID
	}

	// Build update request
	updateReq := &v1.UpdateAgentRequest{
		Id: currentAgent.Id,
	}

	// Check what fields have changed and set them in the update request
	if editedSpec.Name != currentAgent.Metadata.Name {
		updateReq.Name = &editedSpec.Name
	}
	if editedSpec.Description != currentAgent.Metadata.Description {
		updateReq.Description = &editedSpec.Description
	}
	if editedSpec.Instructions != currentAgent.Spec.Instructions {
		updateReq.Instructions = &editedSpec.Instructions
	}
	if modelID != currentAgent.Spec.ModelId {
		updateReq.ModelId = &modelID
	}

	// Apply the update
	_, err := client.Agent().UpdateAgent(ctx, &connect.Request[v1.UpdateAgentRequest]{
		Msg: updateReq,
	})

	return err
}

func indentString(s, indent string) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = indent + line
		}
	}
	return strings.Join(lines, "\n")
}
