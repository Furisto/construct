package cmd

import (
	"errors"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var modelProviderCmd = &cobra.Command{
	Use:   "modelprovider",
	Short: "Manage model providers",
}

func init() {
	rootCmd.AddCommand(modelProviderCmd)
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
	switch v {
	case "openai", "anthropic":
		*e = ModelProviderType(v)
		return nil
	default:
		return errors.New(`must be one of "openai" or "anthropic"`)
	}
}

func (e *ModelProviderType) Type() string {
	return "modelprovider"
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
