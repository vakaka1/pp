package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:     "delete [name]",
	Aliases: []string{"rm", "remove"},
	Short:   "Delete an imported client config",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		path, err := resolveConfigPath(name)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		if err := os.Remove(path); err != nil {
			fmt.Printf("Failed to delete config: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully deleted %s\n", path)
	},
}
