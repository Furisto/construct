package tool


// # Description
// Initiates interactive communication with the user to gather additional information, clarification, or specific details needed to complete a task effectively. This tool enables the agent to resolve ambiguities and make informed decisions by directly querying the user for input. It serves as a bridge between the agent's understanding and the user's intent, ensuring accurate task execution.

// # Parameters
// - **question**: (required) The specific question to ask the user. Should be clear, concise, and directly related to the information gap that needs to be filled. Frame questions to elicit actionable responses that will help you proceed with the task.
// - **options**: (optional) An array of 2-5 predefined answer choices for the user to select from. Each option should be a descriptive string representing a viable answer. This parameter streamlines user interaction by providing quick selection rather than requiring typed responses.

// # Expected Output
// Returns an object containing the user's response:
// %[1]s
// {
//   "user_response": "The user's answer as a string",
//   "selected_option": "If options were provided and user selected one, this contains the selected option"
// }
// %[1]s

// The response format may vary depending on whether the user provides a free-form answer or selects from provided options.

// # CRITICAL REQUIREMENTS
// - **Judicious Usage**: Use this tool sparingly to maintain conversation flow and avoid excessive back-and-forth exchanges
// - **Specific Questions**: Ask targeted, specific questions rather than broad or vague inquiries
// - **Actionable Information**: Focus on gathering information that directly impacts your ability to complete the task
// - **Clear Options**: When providing options, ensure they are mutually exclusive and comprehensive
// - **Option Limitations**: Provide 2-5 options maximum - too many choices can overwhelm the user
// - **No Mode Toggle Options**: Never include options that ask users to switch to different operational modes, as these must be handled manually by the user
// - **Context Awareness**: Frame questions with sufficient context so users understand why the information is needed

// # When to use
// - **Ambiguous Requirements**: When task specifications are unclear or could be interpreted multiple ways
// - **Missing Information**: When critical details needed for task completion are absent
// - **Decision Points**: When multiple valid approaches exist and user preference is needed
// - **Validation Needs**: When confirmation of assumptions or understanding is required
// - **Parameter Clarification**: When function parameters or configuration options need user input
// - **Error Resolution**: When encountering issues that require user guidance to resolve

// # Common Errors and Solutions
// - **"Too many options provided"**: Limit options array to 2-5 items maximum
// - **"Vague question"**: Ensure questions are specific and actionable rather than open-ended
// - **"Excessive questioning"**: Avoid asking multiple questions in succession; gather sufficient context first
// - **"Invalid option format"**: Ensure each option is a string and represents a complete, understandable choice

// # Usage Examples

// ## Basic question without options
// %[1]s
// ask_user({
//   question: "What programming language should I use for this API - Python with FastAPI or Node.js with Express?"
// })
// %[1]s

// ## Question with predefined options
// %[1]s
// ask_user({
//   question: "Which database setup do you prefer for this project?",
//   options: [
//     "SQLite for local development and testing",
//     "PostgreSQL for production-ready setup",
//     "MySQL for compatibility with existing systems",
//     "MongoDB for document-based data structure"
//   ]
// })
// %[1]s

// ## Clarification for ambiguous requirements
// %[1]s
// ask_user({
//   question: "When you mentioned 'responsive design', do you need mobile-first approach or desktop-first?",
//   options: [
//     "Mobile-first (optimize for phones, then scale up)",
//     "Desktop-first (optimize for desktop, then scale down)"
//   ]
// })
// %[1]s