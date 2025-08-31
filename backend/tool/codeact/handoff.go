package codeact

import (
	"fmt"

	"github.com/grafana/sobek"

	"github.com/furisto/construct/backend/tool/communication"
)

const handoffDescription = `
## Description
Delegates the current task to the specified agent.

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

func NewHandoffTool() Tool {
	return NewOnDemandTool(
		"handoff",
		fmt.Sprintf(handoffDescription, "```", "`"),
		handoffInput,
		handoffHandler,
	)
}

func handoffInput(session *Session, args []sobek.Value) (any, error) {
	if len(args) < 1 {
		return nil, nil
	}

	agent := args[0].String()
	var handoverMessage string
	if len(args) > 1 && args[1] != sobek.Undefined() {
		handoverMessage = args[1].String()
	}

	return &communication.HandoffInput{
		TaskID:          session.Task.ID,
		CurrentAgentID:  session.AgentID,
		RequestedAgent:  agent,
		HandoverMessage: handoverMessage,
	}, nil
}

func handoffHandler(session *Session) func(call sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		rawInput, err := handoffInput(session, call.Arguments)
		if err != nil {
			session.Throw(err)
		}
		input := rawInput.(*communication.HandoffInput)

		err = communication.Handoff(session.Context, session.Memory, input)
		if err != nil {
			session.Throw(err)
		}

		return session.VM.ToValue(sobek.Undefined())
	}
}
