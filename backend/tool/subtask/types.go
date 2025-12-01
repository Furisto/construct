package subtask

import (
	"github.com/furisto/construct/backend/memory/schema/types"
)

type SpawnTaskInput struct {
	Agent  string
	Prompt string
}

type SpawnTaskResult struct {
	TaskID string `json:"task_id"`
}

type SendMessageInput struct {
	To      string
	Content *types.MessageContent
}

type SendMessageResult struct {
	Delivered bool   `json:"delivered"`
	Error     string `json:"error,omitempty"`
}

type AwaitTasksInput struct {
	TaskIDs []string
	Timeout int
}

type AwaitTasksResult struct {
	Results []TaskResult `json:"results"`
}

type TaskResult struct {
	TaskID   string                  `json:"task_id"`
	Messages []*types.MessageContent `json:"messages"`
}
