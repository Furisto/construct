package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

type configDescription struct {
	Type        string
	Description string
	Example     string
	Default     string
}

var configDescriptions = map[string]configDescription{
	"defaults.agent": {
		Description: "Specifies the default agent to use when running `construct new` without the\n  --agent flag. This allows you to set a preferred agent for new conversations.",
		Type:        "String (Agent Name or ID)",
		Example:     "construct config set defaults.agent \"my-favorite-agent\"",
	},
}

func NewConfigExplainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "explain <key>",
		Short: "Explain a configuration value",
		Long:  `The "explain" command allows you to explain a configuration value`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			description, ok := configDescriptions[key]
			if !ok {
				return fmt.Errorf("unknown configuration key: %s", key)
			}

			defaultValue := description.Default
			if defaultValue == "" {
				defaultValue = "(none)"
			}

			fmt.Printf("%s\n\n", key)
			fmt.Printf("  %s\n\n", description.Description)
			fmt.Printf("  Type                         Default\n")
			fmt.Printf("  %-28s %s\n\n", description.Type, defaultValue)
			fmt.Printf("  Example: %s\n", description.Example)

			return nil
		},
	}

	return cmd
}
