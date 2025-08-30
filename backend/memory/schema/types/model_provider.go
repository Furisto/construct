package types

type ModelProviderType string

const (
	ModelProviderTypeAnthropic ModelProviderType = "anthropic"
	ModelProviderTypeOpenAI    ModelProviderType = "openai"
	ModelProviderTypeGemini    ModelProviderType = "gemini"
)

func (p ModelProviderType) Values() []string {
	return []string{
		string(ModelProviderTypeAnthropic),
		string(ModelProviderTypeOpenAI),
		string(ModelProviderTypeGemini),
	}
}
