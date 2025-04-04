package cmd

import "github.com/spf13/cobra"

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		return RunAgent(cmd.Context())
	},
}

func init() {
	rootCmd.AddCommand(daemonCmd)
}
