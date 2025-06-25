package memory

import (
	"errors"
	"strings"
)

func SanitizeError(err error) error {
	return errors.New(strings.ReplaceAll(err.Error(), "memory: ", ""))
}
