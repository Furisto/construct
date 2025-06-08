package cmd

import (
	"fmt"
	"io"

	"connectrpc.com/connect"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

type agentCreateOptions struct {
	Description  string
	SystemPrompt string
	PromptFile   string
	PromptStdin  bool
	Model        string
}

func NewAgentCreateCmd() *cobra.Command {
	var options agentCreateOptions

	var cmd = &cobra.Command{
		Use:   "create <name> [flags]",
		Short: "Create a new agent",
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

			systemPrompt, err := getSystemPrompt(&options, cmd.InOrStdin(), getFileSystem(cmd.Context()))
			if err != nil {
				return err
			}

			client := getAPIClient(cmd.Context())

			_, err = uuid.Parse(options.Model)
			if err != nil {
				modelID, err := getModelID(cmd.Context(), client, options.Model)
				if err != nil {
					return err
				}
				options.Model = modelID
			}

			agentResp, err := client.Agent().CreateAgent(cmd.Context(), &connect.Request[v1.CreateAgentRequest]{
				Msg: &v1.CreateAgentRequest{
					Name:         name,
					Description:  options.Description,
					Instructions: systemPrompt,
					ModelId:      options.Model,
				},
			})

			if err != nil {
				return fmt.Errorf("failed to create agent: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), agentResp.Msg.Agent.Id)
			return nil
		},
	}

	cmd.Flags().StringVarP(&options.Description, "description", "d", "", "Description of the agent (optional)")
	cmd.Flags().StringVarP(&options.SystemPrompt, "prompt", "p", "", "System prompt that defines the agent's behavior")
	cmd.Flags().StringVar(&options.PromptFile, "prompt-file", "", "Read system prompt from file")
	cmd.Flags().BoolVar(&options.PromptStdin, "prompt-stdin", false, "Read system prompt from stdin")
	cmd.Flags().StringVarP(&options.Model, "model", "m", "", "AI model to use (e.g. gpt-4o, claude-4 or model ID) (required)")

	cmd.MarkFlagRequired("model")

	return cmd
}

func getSystemPrompt(options *agentCreateOptions, stdin io.Reader, fs *afero.Afero) (string, error) {
	promptSources := 0

	if options.SystemPrompt != "" {
		promptSources++
	}
	if options.PromptFile != "" {
		promptSources++
	}
	if options.PromptStdin {
		promptSources++
	}

	if promptSources == 0 {
		return "", fmt.Errorf("system prompt is required (use --prompt, --prompt-file, or --prompt-stdin)")
	}
	if promptSources > 1 {
		return "", fmt.Errorf("only one prompt source can be specified (--prompt, --prompt-file, or --prompt-stdin)")
	}

	// Inline prompt
	if options.SystemPrompt != "" {
		return options.SystemPrompt, nil
	}

	// From file
	if options.PromptFile != "" {
		content, err := fs.ReadFile(options.PromptFile)
		if err != nil {
			return "", fmt.Errorf("failed to read prompt file %s: %w", options.PromptFile, err)
		}
		if len(content) == 0 {
			return "", fmt.Errorf("prompt file %s is empty", options.PromptFile)
		}
		return string(content), nil
	}

	// From stdin
	if options.PromptStdin {
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
