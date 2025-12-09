package codeact

type InterpreterToolResult struct {
	ID            string     `json:"id"`
	Output        string     `json:"output"`
	FunctionCalls []ToolCall `json:"function_calls"`
	Error         string     `json:"error"`
}

func (r *InterpreterToolResult) Kind() string {
	return "interpreter"
}
