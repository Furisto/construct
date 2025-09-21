package cmd

import (
	"github.com/furisto/construct/shared/config"
	"github.com/spf13/cobra"
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

			if !options.Force && !config.IsLeafValue(value.Raw()) {
				availableKeys := getKeysUnderSection(key)
				if len(availableKeys) > 0 {
					cmd.Printf("You are about to remove the entire '%s' section and all its children:\n%s\n\n", key, formatAvailableKeys(availableKeys))
					if !confirm(cmd.InOrStdin(), cmd.OutOrStdout(), "Are you sure?") {
						return nil
					}
				}
			}

			err = configStore.Delete(key)
			if err != nil {
				return err
			}

			return configStore.Flush(getFileSystem(cmd.Context()))
		},
	}

	cmd.Flags().BoolVarP(&options.Force, "force", "f", false, "Force the removal of the configuration value")

	return cmd
}
