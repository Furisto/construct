package conv

import (
	"fmt"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/schema/types"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func ConvertModelProviderIntoProto(mp *memory.ModelProvider) (*v1.ModelProvider, error) {
	if mp == nil {
		return nil, nil
	}

	protoType, err := ConvertModelProviderTypeToProto(mp.ProviderType)
	if err != nil {
		return nil, err
	}

	return &v1.ModelProvider{
		Metadata: &v1.ModelProviderMetadata{
			Id:           mp.ID.String(),
			CreatedAt:    timestamppb.New(mp.CreateTime),
			UpdatedAt:    timestamppb.New(mp.UpdateTime),
			ProviderType: protoType,
		},
		Spec: &v1.ModelProviderSpec{
			Name:    mp.Name,
			Enabled: mp.Enabled,
		},
	}, nil
}

func ConvertModelProviderTypeToProto(dbType types.ModelProviderType) (v1.ModelProviderType, error) {
	switch dbType {
	case types.ModelProviderTypeAnthropic:
		return v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC, nil
	case types.ModelProviderTypeOpenAI:
		return v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI, nil
	default:
		return v1.ModelProviderType_MODEL_PROVIDER_TYPE_UNSPECIFIED, fmt.Errorf("unsupported provider type: %v", dbType)
	}
}

func ConvertModelProviderTypeToMemory(protoType v1.ModelProviderType) (types.ModelProviderType, error) {
	switch protoType {
	case v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC:
		return types.ModelProviderTypeAnthropic, nil
	case v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI:
		return types.ModelProviderTypeOpenAI, nil
	default:
		return "", fmt.Errorf("unsupported provider type: %v", protoType)
	}
}
