package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"

	"connectrpc.com/connect"
	api "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type AgentEditSpec struct {
	Name         string `yaml:"name"`
	Description  string `yaml:"description"`
	Instructions string `yaml:"instructions"`
	Model        string `yaml:"model"`
}

func NewAgentEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <id-or-name>",
		Short: "Edit an agent in your default editor",
		Long: `Edit an agent's configuration using your default editor ($EDITOR).

This command fetches the current agent configuration and opens it as a YAML
file in your editor. After you save and close the file, any changes will
be applied. If $EDITOR is not set, a common editor (like code, vim, or nano)
will be used.`,
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

			agentID, err := getAgentID(cmd.Context(), client, idOrName)
			if err != nil {
				return fmt.Errorf("failed to resolve agent %s: %w", idOrName, err)
			}

			agentResp, err := client.Agent().GetAgent(cmd.Context(), &connect.Request[v1.GetAgentRequest]{
				Msg: &v1.GetAgentRequest{Id: agentID},
			})
			if err != nil {
				return fmt.Errorf("failed to get agent %s: %w", idOrName, err)
			}

			modelResp, err := client.Model().GetModel(cmd.Context(), &connect.Request[v1.GetModelRequest]{
				Msg: &v1.GetModelRequest{
					Id: agentResp.Msg.Agent.Spec.ModelId,
				},
			})
			if err != nil {
				return fmt.Errorf("failed to get model %s: %w", agentResp.Msg.Agent.Spec.ModelId, err)
			}

			editSpec := &AgentEditSpec{
				Name:         agentResp.Msg.Agent.Spec.Name,
				Description:  agentResp.Msg.Agent.Spec.Description,
				Instructions: agentResp.Msg.Agent.Spec.Instructions,
				Model:        modelResp.Msg.Model.Spec.Name,
			}

			originalSpec := *editSpec

			tempFile, err := createTempYAMLFile(editSpec)
			if err != nil {
				return fmt.Errorf("failed to create temporary file: %w", err)
			}
			defer os.Remove(tempFile)

			if err := openEditor(tempFile); err != nil {
				return fmt.Errorf("failed to open editor: %w", err)
			}

			editedSpec, err := parseYAMLFile(tempFile)
			if err != nil {
				return fmt.Errorf("failed to parse edited content: %w", err)
			}

			if reflect.DeepEqual(originalSpec, *editedSpec) {
				cmd.Println("No changes made.")
				return nil
			}

			if err := applyAgentChanges(cmd.Context(), client, agentResp.Msg.Agent, editedSpec); err != nil {
				return fmt.Errorf("failed to apply changes: %w", err)
			}

			return nil
		},
	}

	return cmd
}

func createTempYAMLFile(spec *AgentEditSpec) (string, error) {
	tempFile, err := os.CreateTemp("", "construct-agent-*.yaml")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	header := `# Please edit the object below. Lines beginning with a '#' will be ignored,
# and an empty file will abort the edit. If an error occurs while saving this file will be
# reopened with the relevant failures.
#
`
	if _, err := tempFile.WriteString(header); err != nil {
		return "", err
	}

	yamlData, err := yaml.Marshal(spec)
	if err != nil {
		return "", err
	}

	if _, err := tempFile.Write(yamlData); err != nil {
		return "", err
	}

	return tempFile.Name(), nil
}

func openEditor(filename string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editors := []string{"code", "code-insiders", "cursor", "subl", "atom", "vim", "nano", "emacs", "vi"}
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

	var cmd *exec.Cmd
	switch filepath.Base(editor) {
	case "code", "code-insiders", "cursor", "subl", "atom":
		cmd = exec.Command(editor, "--wait", filename)
	default:
		cmd = exec.Command(editor, filename)
	}

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
	modelID := editedSpec.Model
	if _, err := uuid.Parse(modelID); err != nil {
		resolvedID, err := getModelID(ctx, client, modelID)
		if err != nil {
			return fmt.Errorf("failed to resolve model %s: %w", modelID, err)
		}
		modelID = resolvedID
	}

	updateReq := &v1.UpdateAgentRequest{
		Id: currentAgent.Metadata.Id,
	}

	if editedSpec.Name != currentAgent.Spec.Name {
		updateReq.Name = &editedSpec.Name
	}
	if editedSpec.Description != currentAgent.Spec.Description {
		updateReq.Description = &editedSpec.Description
	}
	if editedSpec.Instructions != currentAgent.Spec.Instructions {
		updateReq.Instructions = &editedSpec.Instructions
	}
	if modelID != currentAgent.Spec.ModelId {
		updateReq.ModelId = &modelID
	}

	_, err := client.Agent().UpdateAgent(ctx, &connect.Request[v1.UpdateAgentRequest]{
		Msg: updateReq,
	})

	return err
}
