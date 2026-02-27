package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/storacha/smelt/mock-upload-service/pkg/config"
	"github.com/storacha/smelt/mock-upload-service/pkg/server"
	"go.uber.org/zap"
)

var (
	cfgFile string
	cfg     *config.Config
	logger  *zap.Logger
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "mock-upload-service",
		Short: "Mock upload service for Storacha local development",
		Long: `A simplified mock of the Storacha upload service (w3infra) for local
Docker Compose development. Routes blob allocations to Piri nodes
and tracks upload state in memory.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var err error

			// Initialize logger
			logLevel := os.Getenv("LOG_LEVEL")
			if logLevel == "debug" {
				logger, err = zap.NewDevelopment()
			} else {
				logger, err = zap.NewProduction()
			}
			if err != nil {
				return fmt.Errorf("failed to create logger: %w", err)
			}

			// Load configuration
			cfg, err = config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			return nil
		},
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the mock upload service",
		RunE: func(cmd *cobra.Command, args []string) error {
			srv, err := server.New(cfg, logger)
			if err != nil {
				return fmt.Errorf("failed to create server: %w", err)
			}
			return srv.Start()
		},
	}

	rootCmd.AddCommand(serveCmd)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
