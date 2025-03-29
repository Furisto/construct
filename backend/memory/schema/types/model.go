package types

type ModelCapability string

const (
	ModelCapabilityImage            ModelCapability = "image"
	ModelCapabilityComputerUse      ModelCapability = "computer_use"
	ModelCapabilityPromptCache      ModelCapability = "prompt_cache"
	ModelCapabilityExtendedThinking ModelCapability = "extended_thinking"
)

func (c ModelCapability) Values() []ModelCapability {
	return []ModelCapability{
		ModelCapabilityImage,
		ModelCapabilityComputerUse,
		ModelCapabilityPromptCache,
		ModelCapabilityExtendedThinking,
	}
}
