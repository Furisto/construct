package subtask

import (
	"context"
	"fmt"

	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/tool/base"
	"github.com/google/uuid"
)

func SendMessage(ctx context.Context, db *memory.Client, fromTaskID uuid.UUID, toTaskID *uuid.UUID, input *SendMessageInput) (*SendMessageResult, error) {
	if input.To == "" {
		return nil, base.NewCustomError("to is required", []string{
			"Specify the recipient: 'parent' (only supported value currently)",
		})
	}

	if input.To != "parent" {
		return nil, base.NewCustomError("only 'parent' is supported as recipient", []string{
			"You can only send messages to your parent task",
			"Use: send_message({ to: 'parent', content: {...} })",
		})
	}

	if input.Content == nil {
		return nil, base.NewCustomError("content is required", []string{
			"Provide the message content to send",
		})
	}

	if toTaskID == nil {
		return &SendMessageResult{
			Delivered: false,
			Error:     "this task has no parent",
		}, nil
	}

	_, err := memory.Transaction(ctx, db, func(tx *memory.Client) (*SendMessageResult, error) {
		toTask, err := tx.Task.Get(ctx, *toTaskID)
		if err != nil {
			if memory.IsNotFound(err) {
				return &SendMessageResult{
					Delivered: false,
					Error:     fmt.Sprintf("task %s not found", toTaskID.String()),
				}, nil
			}
			return nil, base.NewCustomError(fmt.Sprintf("failed to get parent task: %v", memory.SanitizeError(err)), []string{
				"This is likely a system bug. Ask the user how to proceed",
			})
		}

		_, err = tx.Message.Create().
			SetTaskID(toTask.ID).
			SetSource(types.MessageSourceTask).
			SetFromTaskID(fromTaskID).
			SetContent(input.Content).
			Save(ctx)
		if err != nil {
			return nil, base.NewCustomError(fmt.Sprintf("failed to create message: %v", memory.SanitizeError(err)), []string{
				"This is likely a system bug. Ask the user how to proceed",
			})
		}

		return &SendMessageResult{
			Delivered: true,
		}, nil
	})

	if err != nil {
		return nil, err
	}

	return &SendMessageResult{
		Delivered: true,
	}, nil
}
