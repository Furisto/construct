
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
```
// Current agent decides to handoff to a support specialist
const handoffResult = handoff({
    agent_name: "coder",
    handover_message: "Please start implementing the feature request for the new user dashboard."
})
```
