package conv

import (
	"fmt"

	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/model"
)

func NewModelConverter() *modelConverter {
	return &modelConverter{}
}

type modelConverter struct{}

func (c *modelConverter) ConvertModelCapabilitiesToMemory(capabilities []model.Capability) ([]types.ModelCapability, error) {
	memoryCapabilities := make([]types.ModelCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		memoryCapability, err := c.ConvertModelCapabilityToMemory(capability)
		if err != nil {
			return nil, err
		}
		memoryCapabilities = append(memoryCapabilities, memoryCapability)
	}
	return memoryCapabilities, nil
}

func (c *modelConverter) ConvertModelCapabilityToMemory(m model.Capability) (types.ModelCapability, error) {
	switch m {
	case model.CapabilityImage:
		return types.ModelCapabilityImage, nil
	case model.CapabilityComputerUse:
		return types.ModelCapabilityComputerUse, nil
	case model.CapabilityExtendedThinking:
		return types.ModelCapabilityExtendedThinking, nil
	case model.CapabilityPromptCache:
		return types.ModelCapabilityPromptCache, nil
	}

	return types.ModelCapabilityComputerUse, fmt.Errorf("unknown model capability: %s", m)
}
