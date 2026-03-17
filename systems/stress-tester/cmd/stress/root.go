package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/storacha/smelt/systems/stress-tester/cmd/stress/retrieve"
	"github.com/storacha/smelt/systems/stress-tester/cmd/stress/upload"
)

var (
	cfgFile string

	rootCmd = &cobra.Command{
		Use:   "stress",
		Short: "Stress testing tool for Storacha network",
		Long: `A stress testing service that performs uploads and retrievals
against the Storacha network, tracking state and metrics.

Commands are organized by operation type (upload/retrieve) and mode (burst/continuous/chaos).

Examples:
  # Run upload burst test
  stress upload burst --instance my-test --spaces 10 --uploads 5

  # Run retrieval burst test against uploaded data
  stress retrieve burst --instance my-test --concurrent 10

  # Show upload status for an instance
  stress upload status --instance my-test

  # List all instances
  stress status`,
	}
)

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "",
		"config file (default: /etc/stress/config.yaml)")

	// Add subcommands
	rootCmd.AddCommand(upload.Cmd)
	rootCmd.AddCommand(retrieve.Cmd)

	// Bind environment variables
	viper.SetEnvPrefix("STRESS")
	viper.AutomaticEnv()
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		// Look for config in default locations
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath("/etc/stress")
		viper.AddConfigPath(".")
		viper.AddConfigPath("./config")
	}

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Config file was found but another error occurred
			fmt.Fprintf(os.Stderr, "Error reading config file: %v\n", err)
		}
		// Config file not found; ignore and use defaults + flags
	}
}
