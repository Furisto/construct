package types

type TaskSpec struct {
	Workspace string `json:"workspace,omitempty"`
}

type TaskStatus struct {
	Turns int64 `json:"turns,omitempty"`
	ToolUses map[string]int64 `json:"tool_uses,omitempty"`
}

type TaskUsage struct {
	InputTokens      int64   `json:"input_tokens,omitempty"`
	OutputTokens     int64   `json:"output_tokens,omitempty"`
	CacheWriteTokens int64   `json:"cache_write_tokens,omitempty"`
	CacheReadTokens  int64   `json:"cache_read_tokens,omitempty"`
	Cost             float64 `json:"cost,omitempty"`
}

type TaskPhase string

const (
	TaskPhaseUnspecified TaskPhase = "unspecified"
	TaskPhaseRunning     TaskPhase = "running"
	TaskPhaseAwaiting    TaskPhase = "awaiting"
	TaskPhaseSuspended   TaskPhase = "suspended"
)

func (t TaskPhase) Values() []string {
	return []string{
		string(TaskPhaseUnspecified),
		string(TaskPhaseRunning),
		string(TaskPhaseAwaiting),
		string(TaskPhaseSuspended),
	}
}
