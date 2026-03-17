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

var burstCmd = &cobra.Command{
	Use:   "burst",
	Short: "Run burst mode upload stress test",
	Long: `Run a burst mode upload stress test.

In burst mode, the runner creates a specified number of spaces and uploads
data to each space, then exits. This is useful for generating test data
that can later be retrieved by the retrieve command.

Example:
  stress upload burst --instance my-test --seed 12345 --spaces 10 --uploads 5`,
	RunE: runUploadBurst,
}

func init() {
	// Required flags
	burstCmd.Flags().String("instance", "", "instance name (required)")
	burstCmd.MarkFlagRequired("instance")

	// Optional seed flag (only used when creating new instance)
	burstCmd.Flags().Int64("seed", 0, "seed for reproducible generation (0 = use timestamp)")

	// Burst mode configuration
	burstCmd.Flags().Int("spaces", 5, "number of spaces to create")
	burstCmd.Flags().Int("uploads", 10, "uploads per space")
	burstCmd.Flags().Int("concurrent", 5, "concurrent uploads")
	burstCmd.Flags().String("total-size", "10MB", "total size per upload")

	// Guppy flags
	burstCmd.Flags().String("email", "", "email for guppy login")

	// Debug flag
	burstCmd.Flags().Bool("debug", false, "enable debug logging")

	// Bind flags to viper
	viper.BindPFlag("runner.upload.burst.spaces", burstCmd.Flags().Lookup("spaces"))
	viper.BindPFlag("runner.upload.burst.uploads_per_space", burstCmd.Flags().Lookup("uploads"))
	viper.BindPFlag("runner.upload.burst.concurrent_uploads", burstCmd.Flags().Lookup("concurrent"))
	viper.BindPFlag("runner.upload.burst.total_size", burstCmd.Flags().Lookup("total-size"))
	viper.BindPFlag("guppy.email", burstCmd.Flags().Lookup("email"))
}

func runUploadBurst(cmd *cobra.Command, args []string) error {
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

	slog.Info("starting upload burst stress test",
		"instance", instanceName,
		"seed", seed,
		"spaces", cfg.Runner.Upload.Burst.Spaces,
		"uploads_per_space", cfg.Runner.Upload.Burst.UploadsPerSpace,
		"concurrent", cfg.Runner.Upload.Burst.ConcurrentUploads,
		"total_size", cfg.Runner.Upload.Burst.TotalSize,
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

	// Run the upload burst application
	err = app.RunUploadBurstApp(ctx, cfg, instanceName, seed)

	// Clean up signal handler before returning
	signal.Stop(sigCh)
	cancel()

	return err
}
