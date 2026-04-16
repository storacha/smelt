package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "smelt",
	Short: "Storacha local development environment manager",
}

func Execute() error {
	return rootCmd.Execute()
}
