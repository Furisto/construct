package config

import (
	"fmt"
	"strings"
)

func SupportedKeys() []string {
	return []string{
		// Command
		"cmd",
		"cmd.new",
		"cmd.new.agent",

		"cmd.exec",
		"cmd.exec.agent",
		"cmd.exec.max-turns",

		"cmd.resume",
		"cmd.resume.recent_task_limit",

		// Logging
		"log",
		"log.level",
		"log.file",
		"log.format",

		// Misc
		"editor",
		"output",
		"output.format",
		"output.no-headers",
		"output.wide",
	}
}

func getNestedValue(data map[string]any, key string) (any, bool) {
	keys := strings.Split(key, ".")
	current := data

	for i, k := range keys {
		if value, exists := current[k]; exists {
			if i == len(keys)-1 {
				return value, true
			}

			if nested, ok := value.(map[string]any); ok {
				current = nested
			} else {
				return nil, false
			}
		} else {
			return nil, false
		}
	}

	return nil, false
}

func setNestedValue(data map[string]any, key string, value any) error {
	keys := strings.Split(key, ".")
	current := data

	for i := 0; i < len(keys)-1; i++ {
		k := keys[i]
		if existing, exists := current[k]; exists {
			if nested, ok := existing.(map[string]any); ok {
				current = nested
			} else {
				return fmt.Errorf("key '%s' already exists as a non-object value", strings.Join(keys[:i+1], "."))
			}
		} else {
			newMap := make(map[string]any)
			current[k] = newMap
			current = newMap
		}
	}

	finalKey := keys[len(keys)-1]
	current[finalKey] = value

	return nil
}

func unsetNestedValue(data map[string]any, key string) error {
	keys := strings.Split(key, ".")
	current := data

	for i := 0; i < len(keys)-1; i++ {
		k := keys[i]
		if existing, exists := current[k]; exists {
			if nested, ok := existing.(map[string]any); ok {
				current = nested
			} else {
				return nil
			}
		} else {
			return nil
		}
	}

	finalKey := keys[len(keys)-1]
	delete(current, finalKey)

	cleanupEmptyMaps(data, keys[:len(keys)-1])
	return nil
}

func cleanupEmptyMaps(data map[string]any, keyPath []string) {
	if len(keyPath) == 0 {
		return
	}

	current := data
	for i := 0; i < len(keyPath)-1; i++ {
		if nested, ok := current[keyPath[i]].(map[string]any); ok {
			current = nested
		} else {
			return
		}
	}

	targetKey := keyPath[len(keyPath)-1]
	if targetMap, ok := current[targetKey].(map[string]any); ok && len(targetMap) == 0 {
		delete(current, targetKey)
		cleanupEmptyMaps(data, keyPath[:len(keyPath)-1])
	}
}

func IsLeafValue(value any) bool {
	switch value.(type) {
	case map[string]any:
		return false
	case []any:
		if arr, ok := value.([]any); ok && len(arr) > 0 {
			if _, isMap := arr[0].(map[string]any); isMap {
				return false
			}
		}
		return true
	default:
		return true
	}
}