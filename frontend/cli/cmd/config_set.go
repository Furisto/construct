package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func NewConfigSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Long:  `Set a configuration value.

Sets a persistent configuration key-value pair. Use dot notation for nested keys.`,
		Example: `  # Set the default agent for the 'new' command
  construct config set cmd.new.agent "coder"

  # Set the default output format to JSON
  construct config set output.format "json"`,
		Args:  cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return getSupportedConfigKeys(), cobra.ShellCompDirectiveNoFileComp
			}

			return []string{}, cobra.ShellCompDirectiveDefault
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			value := args[1]
			userInfo := getUserInfo(cmd.Context())

			err := isSupportedKey(key)
			if err != nil {
				return err
			}

			parsedValue, err := parseValue(value)
			if err != nil {
				return fmt.Errorf("invalid value: %w", err)
			}

			constructDir, err := userInfo.ConstructConfigDir()
			if err != nil {
				return fmt.Errorf("failed to retrieve construct config directory: %w", err)
			}

			configFile := filepath.Join(constructDir, "config.yaml")
			fs := getFileSystem(cmd.Context())

			var config map[string]any

			exists, err := fs.Exists(configFile)
			if err != nil {
				return fmt.Errorf("failed to check config file: %w", err)
			}

			if exists {
				content, err := fs.ReadFile(configFile)
				if err != nil {
					return fmt.Errorf("failed to read config file: %w", err)
				}

				if err := yaml.Unmarshal(content, &config); err != nil {
					return fmt.Errorf("failed to parse config file: %w", err)
				}
			} else {
				config = make(map[string]any)
			}

			if isSectionKey(key) {
				availableKeys := getKeysUnderSection(key)
				if len(availableKeys) > 0 {
					return fmt.Errorf("'%s' is a configuration section, not a single value.\nYou can only set a specific key within a section.\n\nAvailable keys under '%s' are:\n%s\n\nExample: construct config set %s %s",
						key, key, formatAvailableKeys(availableKeys), availableKeys[0], "value")
				}
			}

			err = setNestedValue(config, key, parsedValue)
			if err != nil {
				return err
			}

			output, err := MarshalYAMLWithSpacing(config)
			if err != nil {
				return fmt.Errorf("failed to marshal config: %w", err)
			}

			if err := fs.WriteFile(configFile, output, 0644); err != nil {
				return fmt.Errorf("failed to write config file: %w", err)
			}

			return nil
		},
	}

	return cmd
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
