
## Description
Communicates work results to the user, whether tasks are fully completed or partially done. This tool sends a structured message about progress, deliverables, and outcomes. After calling this tool, no additional work can be performed unless the user provides follow-up tasks.

## Parameters
- **summary** (string, required): A clear, concise summary of what was accomplished during the work session. This should highlight the key deliverables, changes made, or outcomes achieved, regardless of completion status.
- **completed** (boolean, required): Indicates whether the assigned task has been fully completed (true) or is still in progress/partial (false).
- **deliverables** (array, optional): An array of strings listing the specific files, features, or outputs that were created or modified during the work. Each item should be a brief description of a concrete deliverable.
- **next_steps** (string, optional): Optional suggestions for what should be done next, or any follow-up actions that might be beneficial to continue the work.

## Expected Output
The tool returns no direct output. The results are communicated to the user through the system.

## CRITICAL RULES
- **Final Action**: This tool MUST be used as the final step to submit your work
- **Session Termination**: After calling this tool, the work session ends and no additional work can be performed
- **Single Use Only**: This tool can only be called once per interpreter run - plan accordingly
- **Complete Information**: Include all relevant information in one call, as you cannot supplement it later

## Usage Examples

### Completed implementation
```
submit_report({
  summary: "Successfully implemented user authentication system with login, registration, and password reset functionality.",
  completed: true,
  deliverables: [
    "User model with email/password authentication",
    "Login and registration API endpoints", 
    "Password reset workflow",
    "Frontend authentication components",
    "Unit and integration tests"
  ],
  next_steps: "You can now test the authentication system by running 'npm start' and navigating to /login"
})
```

### Partial progress due to time constraints
```
submit_report({
  summary: "Made significant progress on the user authentication system. Completed backend API and database models, but frontend components still need implementation.",
  completed: false,
  deliverables: [
    "User authentication database schema",
    "Login and registration API endpoints",
    "Password hashing and validation logic",
    "Basic authentication middleware"
  ],
  next_steps: "Next: implement frontend login/registration forms and integrate with the backend APIs"
})
```

### Analysis results
```
submit_report({
  summary: "Completed security audit of the application and identified key vulnerabilities with recommendations.",
  completed: true,
  deliverables: [
    "Security audit report (security-audit.md)",
    "List of 12 identified vulnerabilities with severity ratings",
    "Prioritized remediation plan",
    "Updated security guidelines document"
  ]
})
```
