package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewContextUseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use <name>",
		Short: "Switch to a different context",
		Args:  cobra.ExactArgs(1),
		Example: `  # Switch to the 'ec2-dev' context
  construct context use ec2-dev`,
		RunE: func(cmd *cobra.Command, args []string) error {
			contextName := args[0]
			contextManager := getContextManager(cmd.Context())

			if contextName == "-" {
				contexts, err := contextManager.LoadContext()
				if err != nil {
					return fmt.Errorf("failed to load contexts: %w", err)
				}

				if contexts.PreviousContext == "" {
					return fmt.Errorf("no previous context found")
				}

				contextName = contexts.PreviousContext
			}

			err := contextManager.SetCurrentContext(contextName)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Switched to context %q\n", contextName)
			return nil
		},
	}

	return cmd
}
