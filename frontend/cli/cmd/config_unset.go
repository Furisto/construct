package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type ConfigUnsetOptions struct {
	Force bool
}

func NewConfigUnsetCmd() *cobra.Command {
	options := ConfigUnsetOptions{}

	cmd := &cobra.Command{
		Use:   "unset <key>",
		Short: "Unset a configuration value",
		Long:  `The "unset" command allows you to unset a configuration value`,
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return getSupportedConfigKeys(), cobra.ShellCompDirectiveNoFileComp
			}

			return []string{}, cobra.ShellCompDirectiveDefault
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			userInfo := getUserInfo(cmd.Context())

			err := isSupportedKey(key)
			if err != nil {
				return err
			}

			constructDir, err := userInfo.ConstructConfigDir()
			if err != nil {
				return fmt.Errorf("failed to retrieve construct config directory: %w", err)
			}

			configFile := filepath.Join(constructDir, "config.yaml")
			fs := getFileSystem(cmd.Context())

			exists, err := fs.Exists(configFile)
			if err != nil {
				return fmt.Errorf("failed to check config file: %w", err)
			}

			if !exists {
				return nil
			}

			content, err := fs.ReadFile(configFile)
			if err != nil {
				return fmt.Errorf("failed to read config file: %w", err)
			}

			var config map[string]any
			if err := yaml.Unmarshal(content, &config); err != nil {
				return fmt.Errorf("failed to parse config file: %w", err)
			}

			value, found := getNestedValue(config, key)
			if !found {
				return nil
			}

			if !options.Force && !isLeafValue(value) {
				availableKeys := getKeysUnderSection(key)
				if len(availableKeys) > 0 {
					cmd.Printf("You are about to remove the entire '%s' section and all its children:\n%s\n\n", key, formatAvailableKeys(availableKeys))
					if !confirm(cmd.InOrStdin(), cmd.OutOrStdout(), "Are you sure?") {
						return nil
					}
				}
			}

			err = unsetNestedValue(config, key)
			if err != nil {
				return err
			}

			content, err = MarshalYAMLWithSpacing(config)
			if err != nil {
				return fmt.Errorf("failed to marshal config: %w", err)
			}

			if err := fs.WriteFile(configFile, content, 0600); err != nil {
				return fmt.Errorf("failed to write config file: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&options.Force, "force", "f", false, "Force the removal of the configuration value")

	return cmd
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

func isLeafValue(value any) bool {
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
