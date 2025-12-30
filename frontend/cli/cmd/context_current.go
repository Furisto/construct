package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewContextCurrentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "current",
		Short: "Display the current context name",
		Example: `  # Show current context
  construct context current`,
		RunE: func(cmd *cobra.Command, args []string) error {
			contextManager := getContextManager(cmd.Context())

			endpointContexts, err := contextManager.LoadContext()
			if err != nil {
				return fmt.Errorf("failed to load contexts: %w", err)
			}

			if endpointContexts.CurrentContext == "" {
				return fmt.Errorf("no current context set")
			}

			fmt.Fprintln(cmd.OutOrStdout(), endpointContexts.CurrentContext)
			return nil
		},
	}

	return cmd
}
