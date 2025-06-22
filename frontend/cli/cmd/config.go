package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sahilm/fuzzy"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config",
		Short:   "Manage configuration",
		GroupID: "system",
	}

	cmd.AddCommand(NewConfigSetCmd())
	cmd.AddCommand(NewConfigGetCmd())
	cmd.AddCommand(NewConfigUnsetCmd())
	cmd.AddCommand(NewConfigExplainCmd())
	cmd.AddCommand(NewConfigListCmd())

	return cmd
}

func getSupportedConfigKeys() []string {
	return []string{
		// Command
		"cmd",
		"cmd.new",
		"cmd.new.agent",

		"cmd.ask",
		"cmd.ask.agent",
		"cmd.ask.max-turns",

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

func isSupportedKey(key string) error {
	if !supportedKeys(key) {
		suggestions := getSuggestions(key)
		if len(suggestions) > 0 {
			return fmt.Errorf("unsupported configuration key: '%s'\n\nDid you mean one of these?\n%s", key, formatSuggestions(suggestions))
		}
		return fmt.Errorf("unsupported configuration key: '%s'", key)
	}

	return nil
}

func getSuggestions(input string) []string {
	supportedKeys := getSupportedConfigKeys()

	matches := fuzzy.Find(input, supportedKeys)

	var suggestions []string
	for i, match := range matches {
		if i >= 3 {
			break
		}
		suggestions = append(suggestions, match.Str)
	}

	return suggestions
}

func formatSuggestions(suggestions []string) string {
	var formatted []string
	for _, suggestion := range suggestions {
		formatted = append(formatted, fmt.Sprintf(" - %s", suggestion))
	}
	return fmt.Sprintln(strings.Join(formatted, "\n"))
}

func supportedKeys(key string) bool {
	supportedKeys := getSupportedConfigKeys()

	for _, supportedKey := range supportedKeys {
		if supportedKey == key {
			return true
		}
	}

	return false
}

func isSectionKey(key string) bool {
	supportedKeys := getSupportedConfigKeys()

	for _, supportedKey := range supportedKeys {
		if strings.HasPrefix(supportedKey, key+".") {
			return true
		}
	}

	return false
}

func getKeysUnderSection(section string) []string {
	supportedKeys := getSupportedConfigKeys()
	var childKeys []string

	prefix := section + "."
	for _, key := range supportedKeys {
		if strings.HasPrefix(key, prefix) {
			remainder := strings.TrimPrefix(key, prefix)
			if !strings.Contains(remainder, ".") {
				childKeys = append(childKeys, key)
			}
		}
	}

	return childKeys
}

func formatAvailableKeys(keys []string) string {
	var formatted []string
	for _, key := range keys {
		parts := strings.Split(key, ".")
		leafKey := parts[len(parts)-1]
		formatted = append(formatted, fmt.Sprintf(" - %s", leafKey))
	}
	return strings.Join(formatted, "\n")
}

func parseValue(value string) (any, error) {
	if boolVal, err := strconv.ParseBool(value); err == nil {
		return boolVal, nil
	}

	if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
		return intVal, nil
	}

	if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
		return floatVal, nil
	}

	return nil, fmt.Errorf("value must be a boolean, integer or float")
}

func MarshalYAMLWithSpacing(v interface{}) ([]byte, error) {
	data, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	var result []string

	for i, line := range lines {
		if i > 0 && len(line) > 0 && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			result = append(result, "")
		}
		result = append(result, line)
	}

	return []byte(strings.Join(result, "\n")), nil
}
