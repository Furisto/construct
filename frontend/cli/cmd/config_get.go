package cmd

import (
	"fmt"
	"sort"

	"github.com/furisto/construct/shared/config"
	"github.com/spf13/cobra"
)

func NewConfigGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value",
		Example: `  # Get the default agent for the 'new' command
  construct config get cmd.new.agent`,
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return config.SupportedKeys(), cobra.ShellCompDirectiveNoFileComp
			}

			return []string{}, cobra.ShellCompDirectiveDefault
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			configStore := getConfigStore(cmd.Context())

			err := validateConfigKey(key)
			if err != nil {
				return err
			}

			value, found := configStore.Get(key)
			if !found {
				return nil
			}

			if config.IsLeafValue(value.Raw()) {
				fmt.Println(value.Raw())
			} else {
				renderConfigValue(value.Raw(), key)
			}

			return nil
		},
	}

	return cmd
}

func renderConfigValue(value any, prefix string) {
	if m, ok := value.(map[string]any); ok {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := m[k]
			fullKey := prefix + "." + k
			if config.IsLeafValue(v) {
				fmt.Printf("%s: %v\n", fullKey, v)
			} else {
				renderConfigValue(v, fullKey)
			}
		}
	}
}
