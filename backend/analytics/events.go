package analytics

func EmitAgentCreated(client Client, agentID string, agentName string, modelName string) {
	client.Enqueue(Event{
		DistinctId: "user",
		Event:      "agent_created",
		Properties: map[string]interface{}{
			"agent_id":   agentID,
			"agent_name": agentName,
			"model_name": modelName,
		},
	})
}

func EmitAgentUpdated(client Client, agentID string, agentName string, updatedFields []string) {
	client.Enqueue(Event{
		DistinctId: "user",
		Event:      "agent_updated",
		Properties: map[string]interface{}{
			"agent_id":       agentID,
			"agent_name":     agentName,
			"updated_fields": updatedFields,
		},
	})
}

func EmitModelProviderCreated(client Client, modelProviderID string, modelProviderName string, providerType string) {
	client.Enqueue(Event{
		DistinctId: "user",
		Event:      "model_provider_created",
		Properties: map[string]interface{}{
			"model_provider_id":   modelProviderID,
			"model_provider_name": modelProviderName,
			"provider_type":       providerType,
		},
	})
}

func EmitModelCreated(client Client, modelID string, modelName string, modelProviderID string, contextWindow int32) {
	client.Enqueue(Event{
		DistinctId: "user",
		Event:      "model_created",
		Properties: map[string]interface{}{
			"model_id":          modelID,
			"model_name":        modelName,
			"model_provider_id": modelProviderID,
			"context_window":    contextWindow,
		},
	})
}

func EmitTaskCreated(client Client, taskID string, agentID string) {
	client.Enqueue(Event{
		DistinctId: "user",
		Event:      "task_created",
		Properties: map[string]interface{}{
			"task_id":  taskID,
			"agent_id": agentID,
		},
	})
}

func EmitTaskUpdated(client Client, taskID string, updatedFields []string) {
	client.Enqueue(Event{
		DistinctId: "user",
		Event:      "task_updated",
		Properties: map[string]interface{}{
			"task_id":        taskID,
			"updated_fields": updatedFields,
		},
	})
}

func EmitAgentDeleted(client Client, agentID string, agentName string) {
	client.Enqueue(Event{
		DistinctId: "user",
		Event:      "agent_deleted",
		Properties: map[string]interface{}{
			"agent_id":   agentID,
			"agent_name": agentName,
		},
	})
}

func EmitModelProviderDeleted(client Client, modelProviderID string, modelProviderName string, providerType string) {
	client.Enqueue(Event{
		DistinctId: "user",
		Event:      "model_provider_deleted",
		Properties: map[string]interface{}{
			"model_provider_id":   modelProviderID,
			"model_provider_name": modelProviderName,
			"provider_type":       providerType,
		},
	})
}

func EmitModelDeleted(client Client, modelID string, modelName string, modelProviderID string) {
	client.Enqueue(Event{
		DistinctId: "user",
		Event:      "model_deleted",
		Properties: map[string]interface{}{
			"model_id":          modelID,
			"model_name":        modelName,
			"model_provider_id": modelProviderID,
		},
	})
}

func EmitTaskDeleted(client Client, taskID string, agentID string) {
	client.Enqueue(Event{
		DistinctId: "user",
		Event:      "task_deleted",
		Properties: map[string]interface{}{
			"task_id":  taskID,
			"agent_id": agentID,
		},
	})
}

func EmitMessageCreated(client Client, messageID string, taskID string, role string) {
	client.Enqueue(Event{
		DistinctId: "user",
		Event:      "message_created",
		Properties: map[string]interface{}{
			"message_id": messageID,
			"task_id":    taskID,
			"role":       role,
		},
	})
}

func EmitMessageDeleted(client Client, messageID string, taskID string, role string) {
	client.Enqueue(Event{
		DistinctId: "user",
		Event:      "message_deleted",
		Properties: map[string]interface{}{
			"message_id": messageID,
			"task_id":    taskID,
			"role":       role,
		},
	})
}

// User Journey & Engagement Events

func EmitSessionStarted(client Client, sessionType string, agentName string, workspace string, hasExistingFiles bool) {
	client.Enqueue(Event{
		DistinctId: "user",
		Event:      "session_started",
		Properties: map[string]interface{}{
			"session_type":       sessionType, // "new", "resume", "ask"
			"agent_name":         agentName,
			"workspace":          workspace,
			"has_existing_files": hasExistingFiles,
		},
	})
}

func EmitConversationResumed(client Client, taskID string, daysSinceLastActivity int, messageCount int) {
	client.Enqueue(Event{
		DistinctId: "user",
		Event:      "conversation_resumed",
		Properties: map[string]interface{}{
			"task_id":                  taskID,
			"days_since_last_activity": daysSinceLastActivity,
			"message_count":            messageCount,
		},
	})
}

func EmitQuickAsk(client Client, agentName string, maxTurns int, filesIncluded int, hasStdinInput bool, questionLength int) {
	client.Enqueue(Event{
		DistinctId: "user",
		Event:      "quick_ask",
		Properties: map[string]interface{}{
			"agent_name":      agentName,
			"max_turns":       maxTurns,
			"files_included":  filesIncluded,
			"has_stdin_input": hasStdinInput,
			"question_length": questionLength,
		},
	})
}

// Tool Usage & Performance Events

func EmitToolExecuted(client Client, taskID string, toolName string, executionTimeMs int64, success bool, errorType string) {
	client.Enqueue(Event{
		DistinctId: "user",
		Event:      "tool_executed",
		Properties: map[string]interface{}{
			"task_id":           taskID,
			"tool_name":         toolName,
			"execution_time_ms": executionTimeMs,
			"success":           success,
			"error_type":        errorType, // empty if success
		},
	})
}

func EmitCodeInterpreterSession(client Client, taskID string, scriptsExecuted int, errorsEncountered int, linesOfCode int) {
	client.Enqueue(Event{
		DistinctId: "user",
		Event:      "code_interpreter_session",
		Properties: map[string]interface{}{
			"task_id":            taskID,
			"scripts_executed":   scriptsExecuted,
			"errors_encountered": errorsEncountered,
			"lines_of_code":      linesOfCode,
		},
	})
}

// Multi-Agent Collaboration Events

func EmitAgentHandoff(client Client, fromAgentID string, toAgentID string, taskID string, reason string, turnNumber int) {
	client.Enqueue(Event{
		DistinctId: "user",
		Event:      "agent_handoff",
		Properties: map[string]interface{}{
			"from_agent_id": fromAgentID,
			"to_agent_id":   toAgentID,
			"task_id":       taskID,
			"reason":        reason, // "specialization", "user_request", "error_recovery"
			"turn_number":   turnNumber,
		},
	})
}
