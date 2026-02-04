package codeact

type InterpreterToolResult struct {
	ID            string           `json:"id"`
	ProviderKind  string           `json:"provider_kind"`
	Output        string           `json:"output"`
	FunctionCalls []FunctionCall   `json:"function_calls"`
	ToolStats     map[string]int64 `json:"tool_stats,omitempty"`
	Error         string           `json:"error"`
}

func (r *InterpreterToolResult) Kind() string {
	return "interpreter"
}
