package retrieve

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
	Short: "Run burst mode retrieval stress test",
	Long: `Run a burst mode retrieval stress test.

In burst mode, the runner retrieves uploads from the specified instance
that were previously uploaded using "stress upload burst". The retrievals
are performed concurrently up to the specified limit.

Example:
  stress retrieve burst --instance my-test --concurrent 10 --limit 100`,
	RunE: runRetrieveBurst,
}

func init() {
	// Required flags
	burstCmd.Flags().String("instance", "", "instance name to retrieve from (required)")
	burstCmd.MarkFlagRequired("instance")

	// Burst mode configuration
	burstCmd.Flags().Int("concurrent", 10, "concurrent retrievals")
	burstCmd.Flags().Int("limit", 0, "maximum retrievals to perform (0 = all)")
	burstCmd.Flags().String("space", "", "filter by space DID (empty = all spaces)")

	// Guppy flags
	burstCmd.Flags().String("email", "", "email for guppy login")

	// Debug flag
	burstCmd.Flags().Bool("debug", false, "enable debug logging")

	// Bind flags to viper
	viper.BindPFlag("runner.retrieve.burst.concurrent_retrievals", burstCmd.Flags().Lookup("concurrent"))
	viper.BindPFlag("runner.retrieve.burst.limit", burstCmd.Flags().Lookup("limit"))
	viper.BindPFlag("runner.retrieve.burst.space_did", burstCmd.Flags().Lookup("space"))
	viper.BindPFlag("guppy.email", burstCmd.Flags().Lookup("email"))
}

func runRetrieveBurst(cmd *cobra.Command, args []string) error {
	// Set up structured logging
	logLevel := slog.LevelInfo
	if debug, _ := cmd.Flags().GetBool("debug"); debug {
		logLevel = slog.LevelDebug
	}
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	slog.SetDefault(slog.New(handler))

	// Get instance name from flag
	instanceName, _ := cmd.Flags().GetString("instance")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	slog.Info("starting retrieve burst stress test",
		"instance", instanceName,
		"concurrent", cfg.Runner.Retrieve.Burst.ConcurrentRetrievals,
		"limit", cfg.Runner.Retrieve.Burst.Limit,
		"space_did", cfg.Runner.Retrieve.Burst.SpaceDID,
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

	// Run the retrieve burst application
	err = app.RunRetrieveBurstApp(ctx, cfg, instanceName)

	// Clean up signal handler before returning
	signal.Stop(sigCh)
	cancel()

	return err
}
