package codeact

import (
	"context"
	"errors"
	"io"

	"github.com/furisto/construct/backend/event"
	"github.com/furisto/construct/backend/memory"
	"github.com/furisto/construct/shared"
	"github.com/google/uuid"
	"github.com/grafana/sobek"
	"github.com/spf13/afero"
)

type Session struct {
	Context       context.Context
	Task          *Task
	AgentID       uuid.UUID
	VM            *sobek.Runtime
	System        io.Writer
	User          io.Writer
	FS            afero.Fs
	Memory        *memory.Client
	Bus           *event.Bus
	CommandRunner shared.CommandRunner

	CurrentTool string
	values      map[string]any
}

func NewSession(ctx context.Context, task *Task, vm *sobek.Runtime, system io.Writer, user io.Writer, fs afero.Fs, commandRunner shared.CommandRunner) *Session {
	return &Session{
		Context:       ctx,
		Task:          task,
		VM:            vm,
		System:        system,
		User:          user,
		FS:            fs,
		CommandRunner: commandRunner,

		values: make(map[string]any),
	}
}

func NewSessionWithBus(ctx context.Context, task *Task, vm *sobek.Runtime, system io.Writer, user io.Writer, fs afero.Fs, memory *memory.Client, bus *event.Bus, commandRunner shared.CommandRunner) *Session {
	return &Session{
		Context:       ctx,
		Task:          task,
		VM:            vm,
		System:        system,
		User:          user,
		FS:            fs,
		Memory:        memory,
		Bus:           bus,
		CommandRunner: commandRunner,

		values: make(map[string]any),
	}
}

func (s *Session) Throw(err error) {
	var toolErr *ToolError
	if errors.As(err, &toolErr) {
		toolErr.Details["toolName"] = s.CurrentTool
	}

	jsErr := s.VM.NewGoError(err)
	panic(jsErr)
}

func SetValue[T any](s *Session, key string, value T) {
	s.values[key] = value
}

func GetValue[T any](s *Session, key string) (T, bool) {
	value, ok := s.values[key]
	if !ok {
		var zero T
		return zero, false
	}
	return value.(T), true
}

func UnsetValue(s *Session, key string) {
	delete(s.values, key)
}

type Task struct {
	ID               uuid.UUID
	ProjectDirectory string
}

type CodeActToolHandler func(session *Session) func(call sobek.FunctionCall) sobek.Value

type Tool interface {
	Name() string
	Description() string
	Input(session *Session, args []sobek.Value) (any, error)
	ToolHandler(session *Session) func(call sobek.FunctionCall) sobek.Value
}

type onDemandTool struct {
	name        string
	description string
	input       func(session *Session, args []sobek.Value) (any, error)
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

func (t *onDemandTool) Input(session *Session, args []sobek.Value) (any, error) {
	return t.input(session, args)
}

func NewOnDemandTool(name, description string, input func(session *Session, args []sobek.Value) (any, error), handler CodeActToolHandler) Tool {
	return &onDemandTool{
		name:        name,
		description: description,
		input:       input,
		handler:     handler,
	}
}
