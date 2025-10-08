package conv

import (
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/schema/types"
)

func ConvertTaskToProto(t *memory.Task) (*v1.Task, error) {
	spec, err := ConvertTaskSpecToProto(t)
	if err != nil {
		return nil, err
	}

	return &v1.Task{
		Metadata: ConvertTaskMetadataToProto(t),
		Spec:     spec,
		Status:   ConvertTaskStatusToProto(t),
	}, nil
}

func ConvertTaskMetadataToProto(t *memory.Task) *v1.TaskMetadata {
	return &v1.TaskMetadata{
		Id:        t.ID.String(),
		CreatedAt: ConvertTimeToTimestamp(t.CreateTime),
		UpdatedAt: ConvertTimeToTimestamp(t.UpdateTime),
	}
}

func ConvertTaskSpecToProto(t *memory.Task) (*v1.TaskSpec, error) {
	return &v1.TaskSpec{
		AgentId:      strPtr(t.AgentID.String()),
		Workspace:    t.ProjectDirectory,
		DesiredPhase: ConvertTaskPhaseToProto(t.DesiredPhase),
	}, nil
}

func ConvertTaskStatusToProto(t *memory.Task) *v1.TaskStatus {
	usage := &v1.TaskUsage{
		InputTokens:      t.InputTokens,
		OutputTokens:     t.OutputTokens,
		CacheWriteTokens: t.CacheWriteTokens,
		CacheReadTokens:  t.CacheReadTokens,
		Cost:             float64(t.Cost),
		ToolUses:         t.ToolUses,
	}

	return &v1.TaskStatus{
		Usage: usage,
		Phase: ConvertTaskPhaseToProto(t.Phase),
	}
}

func ConvertTaskPhaseToProto(p types.TaskPhase) v1.TaskPhase {
	switch p {
	case types.TaskPhaseAwaiting:
		return v1.TaskPhase_TASK_PHASE_AWAITING
	case types.TaskPhaseRunning:
		return v1.TaskPhase_TASK_PHASE_RUNNING
	case types.TaskPhaseSuspended:
		return v1.TaskPhase_TASK_PHASE_SUSPENDED
	default:
		return v1.TaskPhase_TASK_PHASE_UNSPECIFIED
	}
}
