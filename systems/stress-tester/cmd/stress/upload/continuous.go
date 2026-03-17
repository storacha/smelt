package upload

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/storacha/smelt/systems/stress-tester/internal/app"
	"github.com/storacha/smelt/systems/stress-tester/internal/config"
)

var continuousCmd = &cobra.Command{
	Use:   "continuous",
	Short: "Run continuous upload stress test",
	Long: `Run a continuous upload stress test.

Uploads data continuously until stopped (Ctrl+C) or duration expires.
Requires existing spaces (create with 'stress upload burst' first).

Scale by running multiple container instances, each representing a distinct client.

Example:
  stress upload continuous --instance my-test --interval 1s
  stress upload continuous --instance my-test --interval 500ms --duration 1h
  stress upload continuous --instance my-test --space did:key:z6Mk...`,
	RunE: runUploadContinuous,
}

func init() {
	// Required flags
	continuousCmd.Flags().String("instance", "", "instance name (required)")
	continuousCmd.MarkFlagRequired("instance")

	// Optional seed flag (only used when creating new instance)
	continuousCmd.Flags().Int64("seed", 0, "seed for reproducible generation (0 = use timestamp)")

	// Continuous mode configuration
	continuousCmd.Flags().String("total-size", "10MB", "size per upload")
	continuousCmd.Flags().String("interval", "1s", "time between uploads")
	continuousCmd.Flags().String("duration", "0", "max runtime (0 = forever)")
	continuousCmd.Flags().String("space", "", "pin to specific space DID (optional)")

	// Guppy flags
	continuousCmd.Flags().String("email", "", "email for guppy login")

	// Debug flag
	continuousCmd.Flags().Bool("debug", false, "enable debug logging")

	// Bind flags to viper
	viper.BindPFlag("runner.upload.continuous.total_size", continuousCmd.Flags().Lookup("total-size"))
	viper.BindPFlag("runner.upload.continuous.interval", continuousCmd.Flags().Lookup("interval"))
	viper.BindPFlag("runner.upload.continuous.duration", continuousCmd.Flags().Lookup("duration"))
	viper.BindPFlag("runner.upload.continuous.space_did", continuousCmd.Flags().Lookup("space"))
	viper.BindPFlag("guppy.email", continuousCmd.Flags().Lookup("email"))
}

func runUploadContinuous(cmd *cobra.Command, args []string) error {
	// Set up structured logging
	logLevel := slog.LevelInfo
	if debug, _ := cmd.Flags().GetBool("debug"); debug {
		logLevel = slog.LevelDebug
	}
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	slog.SetDefault(slog.New(handler))

	// Get instance name and seed from flags
	instanceName, _ := cmd.Flags().GetString("instance")
	seed, _ := cmd.Flags().GetInt64("seed")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	slog.Info("starting continuous upload stress test",
		"instance", instanceName,
		"seed", seed,
		"interval", cfg.Runner.Upload.Continuous.Interval,
		"duration", cfg.Runner.Upload.Continuous.Duration,
		"total_size", cfg.Runner.Upload.Continuous.TotalSize,
		"space_did", cfg.Runner.Upload.Continuous.SpaceDID,
	)

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case sig := <-sigCh:
			slog.Info("received signal, shutting down", "signal", sig)
			cancel()
		case <-ctx.Done():
			// Context cancelled, exit goroutine
		}
	}()

	// Run the continuous upload application
	err = app.RunUploadContinuousApp(ctx, cfg, instanceName, seed)

	// Clean up signal handler before returning
	signal.Stop(sigCh)
	cancel()

	return err
}
