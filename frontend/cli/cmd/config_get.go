package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func NewConfigGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value",
		Long:  `The "get" command allows you to get a configuration value`,
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

			constructDir, err := userInfo.ConstructDir()
			if err != nil {
				return fmt.Errorf("failed to get construct directory: %w", err)
			}

			settingsFile := filepath.Join(constructDir, "config.yaml")
			fs := getFileSystem(cmd.Context())

			exists, err := fs.Exists(settingsFile)
			if err != nil {
				return fmt.Errorf("failed to check config file: %w", err)
			}

			if !exists {
				return nil
			}

			content, err := fs.ReadFile(settingsFile)
			if err != nil {
				return fmt.Errorf("failed to read config file: %w", err)
			}

			var settings map[string]any
			if err := yaml.Unmarshal(content, &settings); err != nil {
				return fmt.Errorf("failed to parse config file: %w", err)
			}

			value, found := getNestedValue(settings, key)
			if !found {
				return nil
			}

			if isLeafValue(value) {
				fmt.Println(value)
			} else {
				renderConfigValue(value, key)
			}

			return nil
		},
	}

	return cmd
}

func renderConfigValue(value any, prefix string) {
	if m, ok := value.(map[string]any); ok {
		for k, v := range m {
			fullKey := prefix + "." + k
			if isLeafValue(v) {
				fmt.Printf("%s: %v\n", fullKey, v)
			} else {
				renderConfigValue(v, fullKey)
			}
		}
	}
}
