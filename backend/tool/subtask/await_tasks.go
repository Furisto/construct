package subtask

import (
	"context"
	"fmt"
	"time"

	"github.com/furisto/construct/backend/event"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/message"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/memory/task"
	"github.com/furisto/construct/backend/tool/base"
	"github.com/google/uuid"
)

func AwaitTasks(ctx context.Context, db *memory.Client, bus *event.Bus, currentTaskID uuid.UUID, input *AwaitTasksInput) (*AwaitTasksResult, error) {
	if len(input.TaskIDs) == 0 {
		return nil, base.NewCustomError("task_ids is required", []string{
			"Provide at least one task ID to wait for",
		})
	}

	timeout := 300
	if input.Timeout > 0 {
		timeout = input.Timeout
	}

	taskIDs := make([]uuid.UUID, 0, len(input.TaskIDs))
	for _, idStr := range input.TaskIDs {
		taskID, err := uuid.Parse(idStr)
		if err != nil {
			return nil, base.NewCustomError(fmt.Sprintf("invalid task ID: %s", idStr), []string{
				"Ensure all task IDs are valid UUIDs",
			})
		}
		taskIDs = append(taskIDs, taskID)
	}

	err := validateTaskOwnership(ctx, db, currentTaskID, taskIDs)
	if err != nil {
		return nil, err
	}

	err = waitForTasksCompletion(ctx, db, bus, taskIDs, time.Duration(timeout)*time.Second)
	if err != nil {
		return nil, err
	}

	messages, err := collectTaskMessages(ctx, db, currentTaskID, taskIDs)
	if err != nil {
		return nil, err
	}

	result := &AwaitTasksResult{
		Results: make([]TaskResult, len(taskIDs)),
	}

	for i, taskID := range taskIDs {
		result.Results[i] = TaskResult{
			TaskID:   taskID.String(),
			Messages: messages[taskID],
		}
	}

	return result, nil
}

func validateTaskOwnership(ctx context.Context, db *memory.Client, currentTaskID uuid.UUID, taskIDs []uuid.UUID) error {
	for _, taskID := range taskIDs {
		subtask, err := db.Task.Query().Where(task.IDEQ(taskID)).Only(ctx)
		if err != nil {
			if memory.IsNotFound(err) {
				return base.NewCustomError(fmt.Sprintf("task %s not found", taskID.String()), []string{
					"Ensure the task ID is correct",
				})
			}
			return base.NewCustomError(fmt.Sprintf("failed to query task: %v", memory.SanitizeError(err)), []string{
				"This is likely a system bug. Ask the user how to proceed",
			})
		}

		if subtask.ParentTaskID == nil || *subtask.ParentTaskID != currentTaskID {
			return base.NewCustomError(fmt.Sprintf("task %s is not a child of the current task", taskID.String()), []string{
				"You can only await tasks that were spawned by the current task",
				"Use spawn_task() to create subtasks before awaiting them",
			})
		}
	}

	return nil
}

func waitForTasksCompletion(ctx context.Context, db *memory.Client, bus *event.Bus, taskIDs []uuid.UUID, timeout time.Duration) error {
	completed := make(map[uuid.UUID]bool)
	for _, taskID := range taskIDs {
		completed[taskID] = false
	}

	ch, sub := event.SubscribeChannel[event.TaskEvent](bus, 100, nil)
	defer sub.Unsubscribe()

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	checkCompletion := func() (bool, error) {
		allComplete := true
		for taskID := range completed {
			if completed[taskID] {
				continue
			}

			t, err := db.Task.Query().Where(task.IDEQ(taskID)).Only(timeoutCtx)
			if err != nil {
				return false, base.NewCustomError(fmt.Sprintf("failed to query task: %v", memory.SanitizeError(err)), []string{
					"This is likely a system bug. Ask the user how to proceed",
				})
			}

			if t.Phase == types.TaskPhaseAwaiting {
				completed[taskID] = true
			} else {
				allComplete = false
			}
		}
		return allComplete, nil
	}

	allComplete, err := checkCompletion()
	if err != nil {
		return err
	}
	if allComplete {
		return nil
	}

	for {
		select {
		case <-timeoutCtx.Done():
			incompleteIDs := []string{}
			for taskID, isComplete := range completed {
				if !isComplete {
					incompleteIDs = append(incompleteIDs, taskID.String())
				}
			}
			return base.NewCustomError(fmt.Sprintf("timeout waiting for tasks: %v", incompleteIDs), []string{})
		case evt := <-ch:
			if completed[evt.TaskID] {
				continue
			}

			allComplete, err := checkCompletion()
			if err != nil {
				return err
			}
			if allComplete {
				return nil
			}
		}
	}
}

func collectTaskMessages(ctx context.Context, db *memory.Client, parentTaskID uuid.UUID, taskIDs []uuid.UUID) (map[uuid.UUID][]*types.MessageContent, error) {
	messages, err := db.Message.Query().
		Where(
			message.TaskIDEQ(parentTaskID),
			message.SourceEQ(types.MessageSourceTask),
			message.FromTaskIDIn(taskIDs...),
		).
		Order(message.ByCreateTime()).
		All(ctx)
	if err != nil {
		return nil, base.NewCustomError(fmt.Sprintf("failed to query messages: %v", memory.SanitizeError(err)), []string{
			"This is likely a system bug. Ask the user how to proceed",
		})
	}

	result := make(map[uuid.UUID][]*types.MessageContent)
	for _, taskID := range taskIDs {
		result[taskID] = make([]*types.MessageContent, 0)
	}

	for _, msg := range messages {
		if msg.FromTaskID != nil {
			result[*msg.FromTaskID] = append(result[*msg.FromTaskID], msg.Content)
		}
	}

	return result, nil
}
