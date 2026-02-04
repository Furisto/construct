package conv

import "strings"

func Ptr[T any](v T) *T {
	return &v
}

func FromPtr[T any](v *T) T {
	if v == nil {
		return *new(T)
	}
	return *v
}

func ErrorToString(err error) string {
	if err == nil {
		return ""
	}
	errorMsg := err.Error()

	if strings.Contains(errorMsg, "ReferenceError:") && strings.Contains(errorMsg, "is not defined") {
		errorMsg += "\n\nNote: Variables do not persist across interpreter runs. If you're referencing a variable from a previous execution, you'll need to define it again in this script."
	}

	return errorMsg
}
