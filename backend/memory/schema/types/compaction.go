package types

// TaskSummary represents a structured summary of compacted conversation history.
// Each section captures a specific type of information to ensure critical details
// are preserved across compression cycles.
type TaskSummary struct {
	// SessionIntent captures what the user wants to accomplish - the overall goal
	// and any stated requirements.
	SessionIntent string `json:"session_intent,omitempty"`

	// PlayByPlay is a chronological sequence of major actions taken - a high-level
	// narrative of what happened.
	PlayByPlay []string `json:"play_by_play,omitempty"`

	// ArtifactTrail tracks files created, modified, read, or deleted, mapping
	// file paths to descriptions of what changed.
	ArtifactTrail map[string]string `json:"artifact_trail,omitempty"`

	// Decisions records key decisions made during the session with their rationale.
	Decisions []Decision `json:"decisions,omitempty"`

	// Breadcrumbs contains file paths, function names, error messages, and other
	// identifiers needed to reconstruct context.
	Breadcrumbs []string `json:"breadcrumbs,omitempty"`

	// PendingTasks lists what remains to be done - the current state of the work.
	PendingTasks []string `json:"pending_tasks,omitempty"`
}

// Decision represents a key decision made during a session along with its rationale.
type Decision struct {
	// Decision is a brief description of what was decided.
	Decision string `json:"decision"`

	// Rationale explains why this decision was made.
	Rationale string `json:"rationale,omitempty"`
}