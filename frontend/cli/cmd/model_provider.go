package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	api "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func NewModelProviderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "modelprovider",
		Short: "Configure providers like OpenAI and Anthropic",
		Long: `Manage integrations to AI model providers to access their language models for your agents.

Providers require API credentials and offer different model capabilities. At least one provider must be configured before creating agents.

Supported providers:
- OpenAI: Access to GPT models (gpt-4, gpt-3.5-turbo, etc.)
- Anthropic: Access to Claude models (claude-3-5-sonnet, claude-3-haiku, etc.)`,
		Aliases: []string{"modelproviders", "mp"},
		GroupID: "resource",
	}

	cmd.AddCommand(NewModelProviderCreateCmd())
	cmd.AddCommand(NewModelProviderGetCmd())
	cmd.AddCommand(NewModelProviderListCmd())
	cmd.AddCommand(NewModelProviderDeleteCmd())

	return cmd
}

// https://stackoverflow.com/questions/50824554/permitted-flag-values-for-cobra
type ModelProviderType string

const (
	ModelProviderTypeOpenAI    ModelProviderType = "openai"
	ModelProviderTypeAnthropic ModelProviderType = "anthropic"
	ModelProviderTypeGemini    ModelProviderType = "gemini"
	ModelProviderTypeUnknown   ModelProviderType = "unknown"
)

func (e *ModelProviderType) String() string {
	return string(*e)
}

func (e *ModelProviderType) Set(v string) error {
	modelProviderType, err := ToModelProviderType(v)
	if err != nil {
		return err
	}
	*e = modelProviderType
	return nil
}

func (e *ModelProviderType) Type() string {
	return "modelprovider"
}

type ModelProviderTypes []ModelProviderType

func (e *ModelProviderTypes) String() string {
	var s []string
	for _, v := range *e {
		s = append(s, v.String())
	}
	return strings.Join(s, ",")
}

func (e *ModelProviderTypes) Set(v string) error {
	if strings.Contains(v, ",") {
		for _, v := range strings.Split(v, ",") {
			v = strings.TrimSpace(v)
			modelProviderType, err := ToModelProviderType(v)
			if err != nil {
				return err
			}
			*e = append(*e, modelProviderType)
		}
	} else {
		modelProviderType, err := ToModelProviderType(v)
		if err != nil {
			return err
		}
		*e = append(*e, modelProviderType)
	}
	return nil
}

func (e *ModelProviderTypes) Type() string {
	return "modelproviders"
}

func ToModelProviderType(v string) (ModelProviderType, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "openai":
		return ModelProviderTypeOpenAI, nil
	case "anthropic":
		return ModelProviderTypeAnthropic, nil
	case "gemini":
		return ModelProviderTypeGemini, nil
	default:
		return ModelProviderTypeUnknown, errors.New(`must be one of "openai","anthropic","gemini"`)
	}
}

func (e *ModelProviderType) ToAPI() (v1.ModelProviderType, error) {
	switch *e {
	case ModelProviderTypeOpenAI:
		return v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI, nil
	case ModelProviderTypeAnthropic:
		return v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC, nil
	case ModelProviderTypeGemini:
		return v1.ModelProviderType_MODEL_PROVIDER_TYPE_GEMINI, nil
	default:
		return v1.ModelProviderType_MODEL_PROVIDER_TYPE_UNSPECIFIED, errors.New("invalid model provider type")
	}
}

func ConvertModelProviderTypeToDisplay(modelProviderType v1.ModelProviderType) ModelProviderType {
	switch modelProviderType {
	case v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI:
		return ModelProviderTypeOpenAI
	case v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC:
		return ModelProviderTypeAnthropic
	case v1.ModelProviderType_MODEL_PROVIDER_TYPE_GEMINI:
		return ModelProviderTypeGemini
	}

	return ModelProviderTypeUnknown
}

type ModelProviderDisplay struct {
	Id           string            `json:"id" detail:"default"`
	Name         string            `json:"name" detail:"default"`
	ProviderType ModelProviderType `json:"provider_type" detail:"default"`
	Enabled      bool              `json:"enabled" detail:"full"`
}

func ConvertModelProviderToDisplay(modelProvider *v1.ModelProvider) *ModelProviderDisplay {
	return &ModelProviderDisplay{
		Id:           modelProvider.Metadata.Id,
		Name:         modelProvider.Spec.Name,
		ProviderType: ConvertModelProviderTypeToDisplay(modelProvider.Metadata.ProviderType),
		Enabled:      modelProvider.Spec.Enabled,
	}
}

// getModelProviderID resolves a model provider ID or name to an ID
func getModelProviderID(ctx context.Context, client *api.Client, idOrName string) (string, error) {
	_, err := uuid.Parse(idOrName)
	if err == nil {
		return idOrName, nil
	}

	resp, err := client.ModelProvider().ListModelProviders(ctx, &connect.Request[v1.ListModelProvidersRequest]{
		Msg: &v1.ListModelProvidersRequest{},
	})
	if err != nil {
		return "", fmt.Errorf("failed to list model providers: %w", err)
	}

	var matches []*v1.ModelProvider
	for _, mp := range resp.Msg.ModelProviders {
		if mp.Spec.Name == idOrName {
			matches = append(matches, mp)
		}
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("model provider %s not found", idOrName)
	}

	if len(matches) > 1 {
		return "", fmt.Errorf("multiple model providers found for %s", idOrName)
	}

	return matches[0].Metadata.Id, nil
}
