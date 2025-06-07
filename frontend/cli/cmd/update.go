package cmd

import "github.com/spf13/cobra"



var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of CLI",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("CLI Version: 1.0.0")
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
