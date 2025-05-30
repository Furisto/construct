package tool

import (
	"context"
	"fmt"

	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/backend/memory/agent"
	"github.com/furisto/construct/backend/memory/schema/types"
	"github.com/furisto/construct/backend/tool/codeact"
	"github.com/google/uuid"
	"github.com/grafana/sobek"
)

const handoffDescription = `
## Description
Delegates the current task to the specified agent. It allows for controlled delegation by optionally setting a maximum number of turns for the receiving agent. Once the target agent completes its task, reaches its max_turns limit, or hands off further, control may return to the original calling agent or proceed according to the defined system workflow.

## Parameters
- **agent_name** (string, required): The unique name or identifier of the target agent to which the conversation or task will be handed off. This must be a known and available agent in the system. 
Ensure the agent_name refers to a currently active and resolvable agent in the system. Attempting to hand off to an unknown, disabled, or invalid agent will result in an error.
- **handover_message (string, optional): An optional message to pass to the target agent. This allows you to provide specific instructions for the target agent to begin its work effectively. 
If omitted, the target agent may rely on the existing shared conversation context or its default starting behavior. Provide clear, concise, and sufficient initial_input to the target agent. 
This context is crucial for a seamless transition and helps the target agent understand its task without needing to re-elicit information unnecessarily. 

## Expected Output
The tool call will not return a value. If the input was invalid, the tool will throw an error.

## When to Use
- **Task Specialization**: When a specific part of a user's request or a sub-task is better handled by an agent with specialized skills, knowledge, or tools (e.g., handing off from a coding agent to a debugging agent).
- **Workflow Orchestration**: To construct complex, multi-step processes where different agents are responsible for different stages (e.g., architect → coder → reviewer).
- **Requested by user**: When the user explicitly requests a specific agent to handle the task.

## Usage Examples

### Example 1: Simple handoff with an initial message
%[1]s
// Current agent decides to handoff to a support specialist
const handoffResult = handoff({
    agent_name: "coder",
    handover_message: "Please start implementing the feature request for the new user dashboard."
})
%[1]s
`

type HandoffInput struct {
	TaskID          uuid.UUID
	CurrentAgentID  uuid.UUID
	RequestedAgent  string
	HandoverMessage string
}

func (h *HandoffInput) Validate() error {
	if h.TaskID == uuid.Nil {
		return fmt.Errorf("task_id is required")
	}
	if h.CurrentAgentID == uuid.Nil {
		return fmt.Errorf("current_agent_id is required")
	}
	if h.RequestedAgent == "" {
		return fmt.Errorf("requested_agent is required")
	}
	return nil
}

func NewHandoffTool() codeact.Tool {
	return codeact.NewOnDemandTool(
		"handoff",
		fmt.Sprintf(handoffDescription, "```", "`"),
		handoffHandler,
	)
}

func handoffHandler(session *codeact.Session) func(call sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		agent := call.Argument(0).String()
		var handoverMessage string
		if len(call.Arguments) > 1 && call.Arguments[1] != sobek.Undefined() {
			handoverMessage = call.Argument(1).String()
		} else {
			handoverMessage = ""
		}

		err := handoff(session.Context, session.Memory, &HandoffInput{
			TaskID:          session.TaskID,
			CurrentAgentID:  session.AgentID,
			RequestedAgent:  agent,
			HandoverMessage: handoverMessage,
		})
		if err != nil {
			session.Throw(err)
		}

		return session.VM.ToValue(sobek.Undefined())
	}
}

func handoff(ctx context.Context, db *memory.Client, input *HandoffInput) error {
	if err := input.Validate(); err != nil {
		return err
	}

	_, err := memory.Transaction(ctx, db, func(tx *memory.Client) (*any, error) {
		currentAgent, err := tx.Agent.Query().Where(agent.IDEQ(input.CurrentAgentID)).WithModel().Only(ctx)
		if err != nil {
			return nil, codeact.NewCustomError(fmt.Sprintf("failed to get current agent: %v", memory.SanitizeError(err)), []string{
				"This is likely due to a bug in the system. Retrying this operation probably won't help.",
				"Ask the user how to proceed",
			})
		}

		requestedAgent, err := tx.Agent.Query().Where(agent.NameEQ(input.RequestedAgent)).First(ctx)
		if err != nil {
			if memory.IsNotFound(err) {
				return nil, codeact.NewCustomError(fmt.Sprintf("agent %s does not exist", input.RequestedAgent), []string{
					"Check the agent name and try again",
				})
			}
			return nil, err
		}

		if requestedAgent.ID == currentAgent.ID {
			return nil, codeact.NewCustomError("agent cannot handoff to itself", []string{
				fmt.Sprintf("You are the source %s agent and cannot handoff to yourself", currentAgent.Name),
			})
		}

		task, err := tx.Task.Get(ctx, input.TaskID)
		if err != nil {
			return nil, err
		}

		_, err = task.Update().SetAgent(requestedAgent).Save(ctx)
		if err != nil {
			return nil, err
		}

		message := fmt.Sprintf("The %s agent has performed a handoff to you, the %s agent.\n\n", currentAgent.Name, input.RequestedAgent)
		if len(input.HandoverMessage) > 0 {
			message += fmt.Sprintf("It has left the following instructions for you:\n%s", input.HandoverMessage)
		}

		_, err = tx.Message.Create().
			SetContent(&types.MessageContent{
				Blocks: []types.MessageBlock{
					{
						Kind:    types.MessageBlockKindText,
						Payload: message,
					},
				},
			}).
			SetTask(task).
			SetAgent(currentAgent).
			SetModel(currentAgent.Edges.Model).
			SetSource(types.MessageSourceAssistant).
			Save(ctx)
		return nil, err
	})

	return err
}
