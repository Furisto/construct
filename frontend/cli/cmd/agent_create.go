package cmd

import (
	"context"
	"fmt"
	"io"

	"connectrpc.com/connect"

	api "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

var agentCreateOptions struct {
	Description  string
	SystemPrompt string
	PromptFile   string
	PromptStdin  bool
	Model        string
}

var agentCreateCmd = &cobra.Command{
	Use:   "create <name> [flags]",
	Short: "Create a new AI agent",
	Args:  cobra.ExactArgs(1),
	Example: `  # Create agent with inline prompt
  construct agent create "coder" --prompt "You are a coding assistant" --model "claude-4"

  # Create agent with prompt from file
  construct agent create "sql-expert" --prompt-file ./prompts/sql-expert.txt --model "claude-4"

  # Create agent with prompt from stdin
  echo "You review code" | construct agent create "reviewer" --prompt-stdin --model "gpt-4o"

  # With description
  construct agent create "RFC writer" --prompt "You help with writing" --model "gemini-2.5.pro" --description "RFC writing assistant"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		systemPrompt, err := getSystemPrompt(cmd.InOrStdin(), getFileSystem(cmd.Context()))
		if err != nil {
			return err
		}

		client := getAPIClient(cmd.Context())

		_, err = uuid.Parse(agentCreateOptions.Model)
		if err != nil {
			modelID, err := getModelID(cmd.Context(), client, agentCreateOptions.Model)
			if err != nil {
				return err
			}
			agentCreateOptions.Model = modelID
		}

		agentResp, err := client.Agent().CreateAgent(cmd.Context(), &connect.Request[v1.CreateAgentRequest]{
			Msg: &v1.CreateAgentRequest{
				Name:         name,
				Description:  agentCreateOptions.Description,
				Instructions: systemPrompt,
				ModelId:      agentCreateOptions.Model,
			},
		})

		if err != nil {
			return fmt.Errorf("failed to create agent: %w", err)
		}

		fmt.Fprintln(cmd.OutOrStdout(), agentResp.Msg.Agent.Id)
		return nil
	},
}

func getSystemPrompt(stdin io.Reader, fs *afero.Afero) (string, error) {
	promptSources := 0

	if agentCreateOptions.SystemPrompt != "" {
		promptSources++
	}
	if agentCreateOptions.PromptFile != "" {
		promptSources++
	}
	if agentCreateOptions.PromptStdin {
		promptSources++
	}

	if promptSources == 0 {
		return "", fmt.Errorf("system prompt is required (use --prompt, --prompt-file, or --prompt-stdin)")
	}
	if promptSources > 1 {
		return "", fmt.Errorf("only one prompt source can be specified (--prompt, --prompt-file, or --prompt-stdin)")
	}

	// Inline prompt
	if agentCreateOptions.SystemPrompt != "" {
		return agentCreateOptions.SystemPrompt, nil
	}

	// From file
	if agentCreateOptions.PromptFile != "" {
		content, err := fs.ReadFile(agentCreateOptions.PromptFile)
		if err != nil {
			return "", fmt.Errorf("failed to read prompt file %s: %w", agentCreateOptions.PromptFile, err)
		}
		return string(content), nil
	}

	// From stdin
	if agentCreateOptions.PromptStdin {
		content, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read prompt from stdin: %w", err)
		}
		if len(content) == 0 {
			return "", fmt.Errorf("no prompt content received from stdin")
		}
		return string(content), nil
	}

	return "", fmt.Errorf("no prompt source specified")
}

func getModelID(ctx context.Context, client *api.Client, modelName string) (string, error) {
	// todo: consider using fuzzy matching
	modelResp, err := client.Model().ListModels(ctx, &connect.Request[v1.ListModelsRequest]{
		Msg: &v1.ListModelsRequest{
			Filter: &v1.ListModelsRequest_Filter{
				Name: api.Ptr(agentCreateOptions.Model),
			},
		},
	})

	if err != nil {
		return "", fmt.Errorf("failed to list models: %w", err)
	}

	if len(modelResp.Msg.Models) == 0 {
		return "", fmt.Errorf("model %s not found", agentCreateOptions.Model)
	}

	if len(modelResp.Msg.Models) > 1 {
		return "", fmt.Errorf("multiple models found for %s", agentCreateOptions.Model)
	}

	return modelResp.Msg.Models[0].Id, nil
}

func init() {
	agentCreateCmd.Flags().StringVarP(&agentCreateOptions.Description, "description", "d", "", "Description of the agent (optional)")
	agentCreateCmd.Flags().StringVarP(&agentCreateOptions.SystemPrompt, "prompt", "p", "", "System prompt that defines the agent's behavior")
	agentCreateCmd.Flags().StringVar(&agentCreateOptions.PromptFile, "prompt-file", "", "Read system prompt from file")
	agentCreateCmd.Flags().BoolVar(&agentCreateOptions.PromptStdin, "prompt-stdin", false, "Read system prompt from stdin")
	agentCreateCmd.Flags().StringVarP(&agentCreateOptions.Model, "model", "m", "", "AI model to use (e.g. gpt-4o, claude-4 or model ID) (required)")
	agentCreateCmd.MarkFlagRequired("model")

	agentCmd.AddCommand(agentCreateCmd)
}
