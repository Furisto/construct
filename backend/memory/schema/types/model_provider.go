package types

type ModelProviderType string

const (
	ModelProviderTypeAnthropic ModelProviderType = "anthropic"
	ModelProviderTypeOpenAI    ModelProviderType = "openai"
)

func (p ModelProviderType) Values() []string {
	return []string{
		string(ModelProviderTypeAnthropic),
		string(ModelProviderTypeOpenAI),
	}
}
