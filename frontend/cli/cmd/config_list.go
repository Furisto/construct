package cmd

import "github.com/spf13/cobra"

func NewConfigListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List all current configuration values",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	return cmd
}
