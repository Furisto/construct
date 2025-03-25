package tool

import (
	"fmt"
)

type Toolbox struct {
	tools map[string]Tool
}

func NewToolbox() *Toolbox {
	return &Toolbox{
		tools: map[string]Tool{},
	}
}

func (t *Toolbox) AddTool(tool Tool) error {
	if _, ok := t.tools[tool.Name]; ok {
		return fmt.Errorf("tool already exists: %s", tool.Name)
	}
	t.tools[tool.Name] = tool
	return nil
}

func (t *Toolbox) ListTools() []Tool {
	tools := make([]Tool, 0, len(t.tools))
	for _, tool := range t.tools {
		tools = append(tools, tool)
	}
	return tools
}
