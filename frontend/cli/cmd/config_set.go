package cmd

import (
	"fmt"

	"github.com/furisto/construct/shared/config"
	"github.com/spf13/cobra"
)

func NewConfigSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Long: `Set a configuration value.

Sets a persistent configuration key-value pair. Use dot notation for nested keys.`,
		Example: `  # Set the default agent for the 'new' command
  construct config set cmd.new.agent "coder"

  # Set the default output format to JSON
  construct config set output.format "json"`,
		Args: cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return config.SupportedKeys(), cobra.ShellCompDirectiveNoFileComp
			}

			return []string{}, cobra.ShellCompDirectiveDefault
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			value := args[1]
			configStore := getConfigStore(cmd.Context())

			err := validateConfigKey(key)
			if err != nil {
				return err
			}

			parsedValue, err := parseValue(value)
			if err != nil {
				return fmt.Errorf("invalid value: %w", err)
			}

			if isSectionKey(key) {
				availableKeys := getKeysUnderSection(key)
				if len(availableKeys) > 0 {
					return fmt.Errorf("'%s' is a configuration section, not a single value.\nYou can only set a specific key within a section.\n\nAvailable keys under '%s' are:\n%s\n\nExample: construct config set %s %s",
						key, key, formatAvailableKeys(availableKeys), availableKeys[0], "value")
				}
			}

			err = configStore.Set(key, parsedValue)
			if err != nil {
				return err
			}

			return configStore.Flush()
		},
	}

	return cmd
}
