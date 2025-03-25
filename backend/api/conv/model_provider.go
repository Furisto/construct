package conv

import (
	"fmt"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/schema/types"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ModelProviderConverter is an interface for converting between memory.ModelProvider and v1.ModelProvider
type ModelProviderConverter interface {
	// ToProto converts a memory.ModelProvider to a v1.ModelProvider
	ToProto(modelProvider *memory.ModelProvider) (*v1.ModelProvider, error)
	
	// FromProto converts a v1.ModelProvider to a memory.ModelProvider
	// This is not implemented yet as it's not needed for the current use case
	// FromProto(protoMP *v1.ModelProvider) (*memory.ModelProvider, error)
}

// NewModelProviderConverter creates a new instance of ModelProviderConverter
func NewModelProviderConverter() ModelProviderConverter {
	return &modelProviderConverter{}
}

type modelProviderConverter struct{}

// ToProto converts a memory.ModelProvider to a v1.ModelProvider
func (c *modelProviderConverter) ToProto(mp *memory.ModelProvider) (*v1.ModelProvider, error) {
	if mp == nil {
		return nil, nil
	}
	
	protoType, err := ConvertProviderTypeToProto(mp.ProviderType)
	if err != nil {
		return nil, err
	}

	return &v1.ModelProvider{
		Id:           mp.ID.String(),
		Name:         mp.Name,
		ProviderType: protoType,
		Enabled:      mp.Enabled,
		CreatedAt:    timestamppb.New(mp.CreateTime),
		UpdatedAt:    timestamppb.New(mp.UpdateTime),
	}, nil
}

// ConvertProviderTypeToProto converts a provider type from DB to proto
func ConvertProviderTypeToProto(dbType types.ModelProviderType) (v1.ModelProviderType, error) {
	switch dbType {
	case types.ModelProviderTypeAnthropic:
		return v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC, nil
	case types.ModelProviderTypeOpenAI:
		return v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI, nil
	default:
		return v1.ModelProviderType_MODEL_PROVIDER_TYPE_UNSPECIFIED, fmt.Errorf("unsupported provider type: %v", dbType)
	}
}

// ConvertProviderTypeFromProto converts a provider type from proto to DB
func ConvertProviderTypeFromProto(protoType v1.ModelProviderType) (types.ModelProviderType, error) {
	switch protoType {
	case v1.ModelProviderType_MODEL_PROVIDER_TYPE_ANTHROPIC:
		return types.ModelProviderTypeAnthropic, nil
	case v1.ModelProviderType_MODEL_PROVIDER_TYPE_OPENAI:
		return types.ModelProviderTypeOpenAI, nil
	default:
		return "", fmt.Errorf("unsupported provider type: %v", protoType)
	}
}
