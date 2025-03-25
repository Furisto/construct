package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var newOptions struct {
	Socket string
}

var newCmd = &cobra.Command{
	Use:   "new",
	Short: "Start a new conversation",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Hello, World!")
	},
}

func init() {
	rootCmd.AddCommand(newCmd)
	newCmd.Flags().StringVarP(&newOptions.Socket, "socket", "s", "", "The socket to connect to")
}
