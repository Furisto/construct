package analytics

import (
	"github.com/posthog/posthog-go"
)

func EmitAgentCreated(client posthog.Client, agentID string, agentName string, modelID string, modelName string) {
	client.Enqueue(posthog.Capture{
		DistinctId: "user",
		Event:      "agent_created",
		Properties: map[string]interface{}{
			"agent_id":   agentID,
			"agent_name": agentName,
			"model_id":   modelID,
			"model_name": modelName,
		},
	})
}

func EmitModelProviderCreated(client posthog.Client, modelProviderID string, modelProviderName string, providerType string) {
	client.Enqueue(posthog.Capture{
		DistinctId: "user",
		Event:      "model_provider_created",
		Properties: map[string]interface{}{
			"model_provider_id":   modelProviderID,
			"model_provider_name": modelProviderName,
			"provider_type":       providerType,
		},
	})
}

func EmitModelCreated(client posthog.Client, modelID string, modelName string, modelProviderID string, contextWindow int32) {
	client.Enqueue(posthog.Capture{
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

func EmitTaskCreated(client posthog.Client, taskID string, agentID string, projectDirectory string) {
	client.Enqueue(posthog.Capture{
		DistinctId: "user",
		Event:      "task_created",
		Properties: map[string]interface{}{
			"task_id":           taskID,
			"agent_id":          agentID,
			"project_directory": projectDirectory,
		},
	})
}

func EmitAgentDeleted(client posthog.Client, agentID string, agentName string) {
	client.Enqueue(posthog.Capture{
		DistinctId: "user",
		Event:      "agent_deleted",
		Properties: map[string]interface{}{
			"agent_id":   agentID,
			"agent_name": agentName,
		},
	})
}

func EmitModelProviderDeleted(client posthog.Client, modelProviderID string, modelProviderName string, providerType string) {
	client.Enqueue(posthog.Capture{
		DistinctId: "user",
		Event:      "model_provider_deleted",
		Properties: map[string]interface{}{
			"model_provider_id":   modelProviderID,
			"model_provider_name": modelProviderName,
			"provider_type":       providerType,
		},
	})
}

func EmitModelDeleted(client posthog.Client, modelID string, modelName string, modelProviderID string) {
	client.Enqueue(posthog.Capture{
		DistinctId: "user",
		Event:      "model_deleted",
		Properties: map[string]interface{}{
			"model_id":          modelID,
			"model_name":        modelName,
			"model_provider_id": modelProviderID,
		},
	})
}

func EmitTaskDeleted(client posthog.Client, taskID string, agentID string) {
	client.Enqueue(posthog.Capture{
		DistinctId: "user",
		Event:      "task_deleted",
		Properties: map[string]interface{}{
			"task_id":  taskID,
			"agent_id": agentID,
		},
	})
}

func EmitMessageCreated(client posthog.Client, messageID string, taskID string, role string) {
	client.Enqueue(posthog.Capture{
		DistinctId: "user",
		Event:      "message_created",
		Properties: map[string]interface{}{
			"message_id": messageID,
			"task_id":    taskID,
			"role":       role,
		},
	})
}

func EmitMessageDeleted(client posthog.Client, messageID string, taskID string, role string) {
	client.Enqueue(posthog.Capture{
		DistinctId: "user",
		Event:      "message_deleted",
		Properties: map[string]interface{}{
			"message_id": messageID,
			"task_id":    taskID,
			"role":       role,
		},
	})
}


