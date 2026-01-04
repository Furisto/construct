package agent

import (
	"context"
	"encoding/json"

	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/spf13/afero"
)

const CompactionToolName = "record_summary"

type CompactionToolInput struct {
	SessionIntent string            `json:"session_intent" jsonschema:"description=What the user wants to accomplish - the overall goal and any stated requirements"`
	PlayByPlay    []string          `json:"play_by_play" jsonschema:"description=Chronological list of major actions taken"`
	ArtifactTrail map[string]string `json:"artifact_trail" jsonschema:"description=Map of file paths to descriptions of what was done to each file"`
	Decisions     []DecisionInput   `json:"decisions" jsonschema:"description=Key decisions made during the session with rationale"`
	Breadcrumbs   []string          `json:"breadcrumbs" jsonschema:"description=File paths and function names and error messages and other identifiers needed to reconstruct context"`
	PendingTasks  []string          `json:"pending_tasks" jsonschema:"description=What remains to be done - current state of incomplete work"`
}

type DecisionInput struct {
	Decision  string `json:"decision" jsonschema:"description=What was decided"`
	Rationale string `json:"rationale" jsonschema:"description=Why this choice was made"`
}

type CompactionTool struct {
	result *types.TaskSummary
}

func NewCompactionTool() *CompactionTool {
	return &CompactionTool{}
}

func (t *CompactionTool) Name() string {
	return CompactionToolName
}

func (t *CompactionTool) Description() string {
	return `Record a structured summary of the conversation. You MUST call this tool to record your analysis of the conversation.

Analyze the conversation and extract:
- session_intent: The user's goal and requirements
- play_by_play: Chronological actions taken (past tense, focus on actions not discussion)
- artifact_trail: Every file touched with description of changes
- decisions: Choices that affect future work with rationale
- breadcrumbs: Exact identifiers like file paths, function names, error messages, URLs
- pending_tasks: Incomplete work (this replaces previous pending tasks entirely)`
}

func (t *CompactionTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"session_intent": map[string]any{
				"type":        "string",
				"description": "What the user wants to accomplish - the overall goal and any stated requirements",
			},
			"play_by_play": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Chronological list of major actions taken",
			},
			"artifact_trail": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
				"description":          "Map of file paths to descriptions of what was done to each file",
			},
			"decisions": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"decision":  map[string]any{"type": "string", "description": "What was decided"},
						"rationale": map[string]any{"type": "string", "description": "Why this choice was made"},
					},
					"required": []string{"decision"},
				},
				"description": "Key decisions made during the session with rationale",
			},
			"breadcrumbs": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "File paths, function names, error messages, and other identifiers needed to reconstruct context",
			},
			"pending_tasks": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "What remains to be done - current state of incomplete work",
			},
		},
		"required": []string{"session_intent", "play_by_play", "artifact_trail", "decisions", "breadcrumbs", "pending_tasks"},
	}
}

func (t *CompactionTool) Run(ctx context.Context, fs afero.Fs, input json.RawMessage) (string, error) {
	var toolInput CompactionToolInput
	if err := json.Unmarshal(input, &toolInput); err != nil {
		return "", err
	}

	decisions := make([]types.Decision, len(toolInput.Decisions))
	for i, d := range toolInput.Decisions {
		decisions[i] = types.Decision{
			Decision:  d.Decision,
			Rationale: d.Rationale,
		}
	}

	t.result = &types.TaskSummary{
		SessionIntent: toolInput.SessionIntent,
		PlayByPlay:    toolInput.PlayByPlay,
		ArtifactTrail: toolInput.ArtifactTrail,
		Decisions:     decisions,
		Breadcrumbs:   toolInput.Breadcrumbs,
		PendingTasks:  toolInput.PendingTasks,
	}

	return "Summary recorded successfully", nil
}

func (t *CompactionTool) Result() *types.TaskSummary {
	return t.result
}
