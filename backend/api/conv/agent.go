package conv

import (
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/memory"
)

func ConvertAgentToProto(a *memory.Agent) (*v1.Agent, error) {
	spec, err := ConvertAgentSpecToProto(a)
	if err != nil {
		return nil, err
	}

	return &v1.Agent{
		Metadata: ConvertAgentMetadataToProto(a),
		Spec:     spec,
	}, nil
}

func ConvertAgentMetadataToProto(a *memory.Agent) *v1.AgentMetadata {
	return &v1.AgentMetadata{
		Id:        a.ID.String(),
		CreatedAt: ConvertTimeToTimestamp(a.CreateTime),
		UpdatedAt: ConvertTimeToTimestamp(a.UpdateTime),
	}
}

func ConvertAgentSpecToProto(a *memory.Agent) (*v1.AgentSpec, error) {
	if a.Edges.Model == nil {
		return nil, &MissingRelatedEntityError{Entity: "model"}
	}
	return &v1.AgentSpec{
		Name:         a.Name,
		Description:  a.Description,
		Instructions: a.Instructions,
		ModelId:      ConvertUUIDToString(a.Edges.Model.ID),
	}, nil
}
