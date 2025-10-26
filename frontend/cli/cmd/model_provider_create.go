package cmd

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"connectrpc.com/connect"
	"golang.org/x/term"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

type modelProviderCreateOptions struct {
	ApiKey string
	Type   ModelProviderType
	Url    string
}

func NewModelProviderCreateCmd() *cobra.Command {
	var options modelProviderCreateOptions

	cmd := &cobra.Command{
		Use:   "create <name> [flags]",
		Short: "Configure a new model provider integration",
		Args:  cobra.ExactArgs(1),
		Long: `Configure a new model provider integration.

Connects construct to an external AI model provider. This step is required to 
gain access to models. API credentials can be provided interactively, via flags
or environment variables (e.g., $OPENAI_API_KEY, $ANTHROPIC_API_KEY).`,
		Example: `  # Create an OpenAI provider, using the API key from the environment
  export OPENAI_API_KEY="sk-..."
  construct provider create "openai-prod" --type openai

  # Create an Anthropic provider, passing the API key directly
  construct provider create "anthropic-dev" --type anthropic --api-key "sk-ant-..."`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			apiKey, err := getAPIKey(&options, options.Type, name)
			if err != nil {
				return err
			}

			client := getAPIClient(cmd.Context())

			providerType, err := options.Type.ToAPI()
			if err != nil {
				return err
			}

			resp, err := client.ModelProvider().CreateModelProvider(cmd.Context(), &connect.Request[v1.CreateModelProviderRequest]{
				Msg: &v1.CreateModelProviderRequest{
					Name:           name,
					ProviderType:   providerType,
					Authentication: &v1.CreateModelProviderRequest_ApiKey{ApiKey: apiKey},
				},
			})

			if err != nil {
				return fmt.Errorf("failed to create model provider: %w", err)
			}

			cmd.Println(resp.Msg.ModelProvider.Metadata.Id)
			return nil
		},
	}

	cmd.Flags().StringVarP(&options.ApiKey, "api-key", "k", "", "The API key. If omitted, the corresponding environment variable will be used")
	cmd.Flags().VarP(&options.Type, "type", "t", "The type of the model provider (required)")

	cmd.MarkFlagRequired("type")

	return cmd
}

func getAPIKey(options *modelProviderCreateOptions, providerType ModelProviderType, name string) (string, error) {
	// Check command line flag
	if options.ApiKey != "" {
		return options.ApiKey, nil
	}

	// Check environment variable
	envVar, err := APIKeyEnvVar(providerType)
	if err != nil {
		return "", err
	}

	if envKey := os.Getenv(envVar); envKey != "" {
		return envKey, nil
	}

	// Prompt for API key
	displayName, err := getProviderDisplayName(providerType)
	if err != nil {
		return "", err
	}

	fmt.Printf("Enter %s API key for %s: ", displayName, name)
	apiKey, err := readPasswordSecurely()
	if err != nil {
		return "", fmt.Errorf("failed to read API key: %w", err)
	}

	if strings.TrimSpace(apiKey) == "" {
		return "", fmt.Errorf("API key cannot be empty\n\nTip: You can also set the %s environment variable or use the --api-key flag", envVar)
	}

	return apiKey, nil
}

func readPasswordSecurely() (string, error) {
	if !term.IsTerminal(int(syscall.Stdin)) {
		return "", fmt.Errorf("cannot prompt for API key in non-interactive terminal\n\nPlease use --api-key flag or set environment variable")
	}

	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}
	fmt.Println()

	return string(bytePassword), nil
}

func APIKeyEnvVar(providerType ModelProviderType) (string, error) {
	switch providerType {
	case ModelProviderTypeOpenAI:
		return "OPENAI_API_KEY", nil
	case ModelProviderTypeAnthropic:
		return "ANTHROPIC_API_KEY", nil
	case ModelProviderTypeGemini:
		return "GEMINI_API_KEY", nil
	case ModelProviderTypeXAI:
		return "XAI_API_KEY", nil
	default:
		return "", fmt.Errorf("unknown provider type: %s", providerType)
	}
}

func getProviderDisplayName(providerType ModelProviderType) (string, error) {
	switch providerType {
	case ModelProviderTypeOpenAI:
		return "OpenAI", nil
	case ModelProviderTypeAnthropic:
		return "Anthropic", nil
	case ModelProviderTypeGemini:
		return "Gemini", nil
	case ModelProviderTypeXAI:
		return "xAI", nil
	default:
		return "", fmt.Errorf("unknown provider type: %s", providerType)
	}
}
