package tool

import (
	"io"

	"github.com/grafana/sobek"
	"github.com/spf13/afero"
)

type CodeActSession struct {
	VM     *sobek.Runtime
	System io.Writer
	User   io.Writer
	FS     afero.Fs

	CurrentTool string
	values      map[string]any
}

func (s *CodeActSession) Throw(err error) {
	jsErr := s.VM.NewGoError(err)
	panic(jsErr)
}

func SetValue[T any](s *CodeActSession, key string, value T) {
	s.values[key] = value
}

func GetValue[T any](s *CodeActSession, key string) (T, bool) {
	value, ok := s.values[key]
	if !ok {
		return value.(T), false
	}
	return value.(T), true
}

type CodeActToolCallback func(session *CodeActSession) func(call sobek.FunctionCall) sobek.Value

type CodeActTool interface {
	Name() string
	Description() string
	ToolCallback(session *CodeActSession) func(call sobek.FunctionCall) sobek.Value
}

type onDemandTool struct {
	name        string
	description string
	handler     CodeActToolCallback
}

func (t *onDemandTool) Name() string {
	return t.name
}

func (t *onDemandTool) Description() string {
	return t.description
}

func (t *onDemandTool) ToolCallback(session *CodeActSession) func(call sobek.FunctionCall) sobek.Value {
	return t.handler(session)
}

func NewOnDemandTool(name, description string, handler CodeActToolCallback) CodeActTool {
	return &onDemandTool{
		name:        name,
		description: description,
		handler:     handler,
	}
}