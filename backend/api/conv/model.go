package conv

import (
	"fmt"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/model"
	"github.com/googleapis/go-type-adapters/adapters"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func MemoryModelToProto(m *memory.Model) (*v1.Model, error) {
	capabilities := make([]v1.ModelCapability, 0, len(m.Capabilities))
	for _, cap := range m.Capabilities {
		capabilities = append(capabilities, MemoryModelCapabilityToProto(cap))
	}

	pricing := &v1.ModelPricing{
		InputCost:      adapters.Float64ToProtoDecimal(m.InputCost),
		OutputCost:     adapters.Float64ToProtoDecimal(m.OutputCost),
		CacheWriteCost: adapters.Float64ToProtoDecimal(m.CacheWriteCost),
		CacheReadCost:  adapters.Float64ToProtoDecimal(m.CacheReadCost),
	}

	metadata := &v1.ModelMetadata{
		Id:              m.ID.String(),
		CreatedAt:       timestamppb.New(m.CreateTime),
		UpdatedAt:       timestamppb.New(m.UpdateTime),
		ModelProviderId: m.ModelProviderID.String(),
	}

	spec := &v1.ModelSpec{
		Name:          m.Name,
		Capabilities:  capabilities,
		Pricing:       pricing,
		ContextWindow: m.ContextWindow,
		Enabled:       m.Enabled,
	}

	return &v1.Model{
		Metadata: metadata,
		Spec:     spec,
	}, nil
}

func MemoryModelCapabilityToProto(cap types.ModelCapability) v1.ModelCapability {
	switch cap {
	case types.ModelCapabilityImage:
		return v1.ModelCapability_MODEL_CAPABILITY_IMAGE
	case types.ModelCapabilityComputerUse:
		return v1.ModelCapability_MODEL_CAPABILITY_COMPUTER_USE
	case types.ModelCapabilityPromptCache:
		return v1.ModelCapability_MODEL_CAPABILITY_PROMPT_CACHE
	case types.ModelCapabilityExtendedThinking:
		return v1.ModelCapability_MODEL_CAPABILITY_THINKING
	}

	return v1.ModelCapability_MODEL_CAPABILITY_UNSPECIFIED
}

func ProtoModelCapabilityToMemory(cap v1.ModelCapability) (types.ModelCapability, error) {
	switch cap {
	case v1.ModelCapability_MODEL_CAPABILITY_IMAGE:
		return types.ModelCapabilityImage, nil
	case v1.ModelCapability_MODEL_CAPABILITY_COMPUTER_USE:
		return types.ModelCapabilityComputerUse, nil
	case v1.ModelCapability_MODEL_CAPABILITY_PROMPT_CACHE:
		return types.ModelCapabilityPromptCache, nil
	case v1.ModelCapability_MODEL_CAPABILITY_THINKING:
		return types.ModelCapabilityExtendedThinking, nil
	default:
		return "", fmt.Errorf("unknown model capability: %v", cap)
	}
}

func ProtoModelPricingToMemory(pricing *v1.ModelPricing) (float64, float64, float64, float64, error) {
	inputCost, _, err := adapters.ProtoDecimalToFloat64(pricing.InputCost)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	outputCost, _, err := adapters.ProtoDecimalToFloat64(pricing.OutputCost)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	cacheWriteCost, _, err := adapters.ProtoDecimalToFloat64(pricing.CacheWriteCost)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	cacheReadCost, _, err := adapters.ProtoDecimalToFloat64(pricing.CacheReadCost)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	return inputCost, outputCost, cacheWriteCost, cacheReadCost, nil
}

func LLMModelCapabilitiesToMemory(caps []model.Capability) ([]types.ModelCapability, error) {
	capabilities := make([]types.ModelCapability, 0, len(caps))
	for _, cap := range caps {
		switch cap {
		case model.CapabilityImage:
			capabilities = append(capabilities, types.ModelCapabilityImage)
		case model.CapabilityComputerUse:
			capabilities = append(capabilities, types.ModelCapabilityComputerUse)
		case model.CapabilityPromptCache:
			capabilities = append(capabilities, types.ModelCapabilityPromptCache)
		case model.CapabilityExtendedThinking:
			capabilities = append(capabilities, types.ModelCapabilityExtendedThinking)
		default:
			return nil, fmt.Errorf("unknown model capability: %v", cap)
		}
	}

	return capabilities, nil
}
