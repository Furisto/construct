package cmd

import "github.com/spf13/cobra"

func NewDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "daemon",
		Short:   "Run the API server as a persistent service",
		GroupID: "system",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunAgent(cmd.Context())
		},
	}

	return cmd
}
