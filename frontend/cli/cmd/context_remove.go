package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

type contextRemoveOptions struct {
	Force bool
}

func NewContextRemoveCmd() *cobra.Command {
	var options contextRemoveOptions

	cmd := &cobra.Command{
		Use:     "remove <name>... [flags]",
		Short:   "Remove one or more contexts",
		Args:    cobra.MinimumNArgs(1),
		Aliases: []string{"rm"},
		Long: `Remove one or more contexts from the configuration.

This will delete the context configuration and remove any associated 
authentication tokens from the system keyring.`,
		Example: `  # Remove a single context
  construct context remove staging

  # Remove multiple contexts
  construct context remove dev staging test

  # Remove without confirmation
  construct context remove production --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			contextManager := getContextManager(cmd.Context())

			if !options.Force && !confirmDeletion(cmd.InOrStdin(), cmd.OutOrStdout(), "context", args) {
				return nil
			}

			for _, contextName := range args {
				err := contextManager.DeleteContext(contextName)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Context %q removed\n", contextName)
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&options.Force, "force", "f", false, "Skip confirmation prompt")

	return cmd
}
