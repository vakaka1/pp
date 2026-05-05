package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var deleteCfgFile string

var deleteCmd = &cobra.Command{
	Use:     "delete [name]",
	Aliases: []string{"rm", "remove"},
	Short:   "Delete an imported client config",
	Args: func(cmd *cobra.Command, args []string) error {
		if deleteCfgFile != "" {
			if len(args) != 0 {
				return fmt.Errorf("delete accepts either --config or a name, not both")
			}
			return nil
		}
		return cobra.ExactArgs(1)(cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		target := deleteCfgFile
		if target == "" {
			target = args[0]
		}
		path, err := resolveConfigPath(target)
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

func init() {
	deleteCmd.Flags().StringVar(&deleteCfgFile, "config", "", "Config file to delete")
}
