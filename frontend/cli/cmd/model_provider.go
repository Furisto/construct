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
		Short: "Manage model providers",
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
	default:
		return ModelProviderTypeUnknown, errors.New(`must be one of "openai" or "anthropic"`)
	}
}

func (e *ModelProviderType) ToAPI() (v1.ModelProviderType, error) {
	switch *e {
	case ModelProviderTypeOpenAI:
		return v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI, nil
	case ModelProviderTypeAnthropic:
		return v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC, nil
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
	}

	return ModelProviderTypeUnknown
}

type ModelProviderDisplay struct {
	Id           string            `json:"id"`
	Name         string            `json:"name"`
	ProviderType ModelProviderType `json:"provider_type"`
	Enabled      bool              `json:"enabled"`
}

func ConvertModelProviderToDisplay(modelProvider *v1.ModelProvider) *ModelProviderDisplay {
	return &ModelProviderDisplay{
		Id:           modelProvider.Id,
		Name:         modelProvider.Name,
		ProviderType: ConvertModelProviderTypeToDisplay(modelProvider.ProviderType),
		Enabled:      modelProvider.Enabled,
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
		if mp.Name == idOrName {
			matches = append(matches, mp)
		}
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("model provider %s not found", idOrName)
	}

	if len(matches) > 1 {
		return "", fmt.Errorf("multiple model providers found for %s", idOrName)
	}

	return matches[0].Id, nil
}
