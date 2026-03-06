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

var continuousCmd = &cobra.Command{
	Use:   "continuous",
	Short: "Run continuous retrieval stress test",
	Long: `Run a continuous retrieval stress test.

Retrieves data continuously until stopped (Ctrl+C) or duration expires.
Requires existing uploads (create with 'stress upload burst' first).

Scale by running multiple container instances, each representing a distinct client.

Example:
  stress retrieve continuous --instance my-test --interval 1s
  stress retrieve continuous --instance my-test --interval 500ms --duration 1h
  stress retrieve continuous --instance my-test --space did:key:z6Mk...`,
	RunE: runRetrieveContinuous,
}

func init() {
	// Required flags
	continuousCmd.Flags().String("instance", "", "instance name (required)")
	continuousCmd.MarkFlagRequired("instance")

	// Continuous mode configuration
	continuousCmd.Flags().String("interval", "1s", "time between retrievals")
	continuousCmd.Flags().String("duration", "0", "max runtime (0 = forever)")
	continuousCmd.Flags().String("space", "", "filter by specific space DID (optional)")

	// Guppy flags
	continuousCmd.Flags().String("email", "", "email for guppy login")

	// Debug flag
	continuousCmd.Flags().Bool("debug", false, "enable debug logging")

	// Bind flags to viper
	viper.BindPFlag("runner.retrieve.continuous.interval", continuousCmd.Flags().Lookup("interval"))
	viper.BindPFlag("runner.retrieve.continuous.duration", continuousCmd.Flags().Lookup("duration"))
	viper.BindPFlag("runner.retrieve.continuous.space_did", continuousCmd.Flags().Lookup("space"))
	viper.BindPFlag("guppy.email", continuousCmd.Flags().Lookup("email"))
}

func runRetrieveContinuous(cmd *cobra.Command, args []string) error {
	// Set up structured logging
	logLevel := slog.LevelInfo
	if debug, _ := cmd.Flags().GetBool("debug"); debug {
		logLevel = slog.LevelDebug
	}
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	slog.SetDefault(slog.New(handler))

	// Get instance name from flags
	instanceName, _ := cmd.Flags().GetString("instance")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	slog.Info("starting continuous retrieval stress test",
		"instance", instanceName,
		"interval", cfg.Runner.Retrieve.Continuous.Interval,
		"duration", cfg.Runner.Retrieve.Continuous.Duration,
		"space_did", cfg.Runner.Retrieve.Continuous.SpaceDID,
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

	// Run the continuous retrieve application
	err = app.RunRetrieveContinuousApp(ctx, cfg, instanceName)

	// Clean up signal handler before returning
	signal.Stop(sigCh)
	cancel()

	return err
}
