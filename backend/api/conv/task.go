package conv

import (
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/memory"
)

func ConvertTaskToProto(t *memory.Task) (*v1.Task, error) {
	spec, err := ConvertTaskSpecToProto(t)
	if err != nil {
		return nil, err
	}

	return &v1.Task{
		Id:       t.ID.String(),
		Metadata: ConvertTaskMetadataToProto(t),
		Spec:     spec,
		Status:   ConvertTaskStatusToProto(t),
	}, nil
}

func ConvertTaskMetadataToProto(t *memory.Task) *v1.TaskMetadata {
	return &v1.TaskMetadata{
		CreatedAt: ConvertTimeToTimestamp(t.CreateTime),
		UpdatedAt: ConvertTimeToTimestamp(t.UpdateTime),
	}
}

func ConvertTaskSpecToProto(t *memory.Task) (*v1.TaskSpec, error) {
	return &v1.TaskSpec{
		AgentId:          strPtr(t.AgentID.String()),
		ProjectDirectory: t.ProjectDirectory,
	}, nil
}

func ConvertTaskStatusToProto(t *memory.Task) *v1.TaskStatus {
	usage := &v1.TaskUsage{
		InputTokens:      t.InputTokens,
		OutputTokens:     t.OutputTokens,
		CacheWriteTokens: t.CacheWriteTokens,
		CacheReadTokens:  t.CacheReadTokens,
	}

	return &v1.TaskStatus{
		Usage: usage,
	}
}
