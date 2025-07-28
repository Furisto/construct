package shared

import (
	"errors"
	"fmt"
)

type ErrorSource int

const (
	ErrorSourceTool ErrorSource = iota
	ErrorSourceAgent
	ErrorSourceSystem
	ErrorSourceUser
	ErrorSourceUnknown
)

type ConstructError struct {
	Source  ErrorSource
	Message string
	Err     error
}

func Errorf(source ErrorSource, format string, a ...any) *ConstructError {
	return &ConstructError{
		Source:  source,
		Message: fmt.Sprintf(format, a...),
	}
}

func Wrap(source ErrorSource, err error, format string, a ...any) *ConstructError {
	return &ConstructError{
		Source:  source,
		Message: fmt.Sprintf(format, a...),
		Err:     err,
	}
}

func (e *ConstructError) Error() string {
	return e.Err.Error()
}

func (e *ConstructError) Unwrap() error {
	return e.Err
}

func (e *ConstructError) Is(target error) bool {
	return errors.Is(e.Err, target)
}

func (e *ConstructError) As(target interface{}) bool {
	return errors.As(e.Err, target)
}
