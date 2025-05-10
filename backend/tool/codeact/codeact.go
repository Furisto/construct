package codeact

import (
	"io"

	"github.com/grafana/sobek"
	"github.com/spf13/afero"
)

type Session struct {
	VM     *sobek.Runtime
	System io.Writer
	User   io.Writer
	FS     afero.Fs

	CurrentTool string
	values      map[string]any
}

func (s *Session) Throw(err error) {
	jsErr := s.VM.NewGoError(err)
	panic(jsErr)
}

func SetValue[T any](s *Session, key string, value T) {
	s.values[key] = value
}

func GetValue[T any](s *Session, key string) (T, bool) {
	value, ok := s.values[key]
	if !ok {
		return value.(T), false
	}
	return value.(T), true
}

type CodeActToolHandler func(session *Session) func(call sobek.FunctionCall) sobek.Value

type Tool interface {
	Name() string
	Description() string
	ToolHandler(session *Session) func(call sobek.FunctionCall) sobek.Value
}

type onDemandTool struct {
	name        string
	description string
	handler     CodeActToolHandler
}

func (t *onDemandTool) Name() string {
	return t.name
}

func (t *onDemandTool) Description() string {
	return t.description
}

func (t *onDemandTool) ToolHandler(session *Session) func(call sobek.FunctionCall) sobek.Value {
	return t.handler(session)
}

func NewOnDemandTool(name, description string, handler CodeActToolHandler) Tool {
	return &onDemandTool{
		name:        name,
		description: description,
		handler:     handler,
	}
}
