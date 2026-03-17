package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/storacha/smelt/systems/stress-tester/internal/config"
	"github.com/storacha/smelt/systems/stress-tester/internal/generator"
	"github.com/storacha/smelt/systems/stress-tester/internal/guppy"
	"github.com/storacha/smelt/systems/stress-tester/internal/runner"
	"github.com/storacha/smelt/systems/stress-tester/internal/store"
	"github.com/storacha/smelt/systems/stress-tester/internal/telemetry"
)

// RunUploadBurstApp creates and runs the upload burst stress test
func RunUploadBurstApp(ctx context.Context, cfg *config.Config, instanceName string, seedOverride int64) error {
	// Create store
	s, err := store.NewGORMStore(store.StoreConfig{
		Type: cfg.Store.Type,
		Path: cfg.Store.Path,
		DSN:  cfg.Store.DSN,
	})
	if err != nil {
		return fmt.Errorf("failed to create store: %w", err)
	}
	defer s.Close()

	// Create telemetry provider
	provider, err := telemetry.NewProvider(telemetry.Config{
		ServiceName:    cfg.Telemetry.ServiceName,
		PrometheusPort: cfg.Telemetry.PrometheusPort,
		OTLPEndpoint:   cfg.Telemetry.OTLPEndpoint,
	})
	if err != nil {
		return fmt.Errorf("failed to create telemetry provider: %w", err)
	}

	if err := provider.Start(); err != nil {
		return fmt.Errorf("failed to start telemetry: %w", err)
	}
	defer func() {
		// Give telemetry 5 seconds to flush metrics before forcing shutdown
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := provider.Stop(shutdownCtx); err != nil {
			slog.Warn("telemetry shutdown error", "error", err)
		}
	}()

	// Create metrics
	metrics, err := telemetry.NewMetrics(provider)
	if err != nil {
		return fmt.Errorf("failed to create metrics: %w", err)
	}

	// Create guppy client
	guppyClient := guppy.NewCLIClient(
		cfg.Guppy.BinaryPath,
		cfg.Guppy.ConfigPath,
		cfg.Guppy.Email,
	)

	// Get or create instance
	// Use provided seed override, or fall back to config seed, or use timestamp
	seed := seedOverride
	if seed == 0 {
		seed = cfg.Generator.Seed
	}
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	instance, created, err := s.GetOrCreateInstance(ctx, instanceName, seed)
	if err != nil {
		return fmt.Errorf("failed to get or create instance: %w", err)
	}

	if created {
		slog.Info("created new instance",
			"name", instanceName,
			"seed", instance.Seed,
		)
	} else {
		slog.Info("joined existing instance",
			"name", instanceName,
			"seed", instance.Seed,
		)
		if seedOverride != 0 && seedOverride != instance.Seed {
			slog.Warn("seed override ignored - instance already exists with different seed",
				"provided_seed", seedOverride,
				"instance_seed", instance.Seed,
			)
		}
	}

	// Create generator with instance's seed
	gen, err := generator.NewGenerator(generator.Config{
		Seed:        instance.Seed,
		MinFileSize: cfg.Generator.MinFileSize,
		MaxFileSize: cfg.Generator.MaxFileSize,
		BaseDir:     "/tmp",
	})
	if err != nil {
		return fmt.Errorf("failed to create generator: %w", err)
	}

	// Create upload burst runner
	uploadRunner, err := runner.NewUploadBurstRunner(
		instance.ID,
		cfg.Runner.Upload.Burst,
		s,
		guppyClient,
		metrics,
		gen,
		cfg.Guppy.Email,
	)
	if err != nil {
		return fmt.Errorf("failed to create upload runner: %w", err)
	}

	// Run the stress test
	return uploadRunner.Run(ctx)
}

// RunRetrieveBurstApp creates and runs the retrieve burst stress test
func RunRetrieveBurstApp(ctx context.Context, cfg *config.Config, instanceName string) error {
	// Create store
	s, err := store.NewGORMStore(store.StoreConfig{
		Type: cfg.Store.Type,
		Path: cfg.Store.Path,
		DSN:  cfg.Store.DSN,
	})
	if err != nil {
		return fmt.Errorf("failed to create store: %w", err)
	}
	defer s.Close()

	// Create telemetry provider
	provider, err := telemetry.NewProvider(telemetry.Config{
		ServiceName:    cfg.Telemetry.ServiceName,
		PrometheusPort: cfg.Telemetry.PrometheusPort,
		OTLPEndpoint:   cfg.Telemetry.OTLPEndpoint,
	})
	if err != nil {
		return fmt.Errorf("failed to create telemetry provider: %w", err)
	}

	if err := provider.Start(); err != nil {
		return fmt.Errorf("failed to start telemetry: %w", err)
	}
	defer func() {
		// Give telemetry 5 seconds to flush metrics before forcing shutdown
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := provider.Stop(shutdownCtx); err != nil {
			slog.Warn("telemetry shutdown error", "error", err)
		}
	}()

	// Create metrics
	metrics, err := telemetry.NewMetrics(provider)
	if err != nil {
		return fmt.Errorf("failed to create metrics: %w", err)
	}

	// Create guppy client
	guppyClient := guppy.NewCLIClient(
		cfg.Guppy.BinaryPath,
		cfg.Guppy.ConfigPath,
		cfg.Guppy.Email,
	)

	// Get instance (must exist for retrieval)
	instance, err := s.GetInstance(ctx, instanceName)
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}
	if instance == nil {
		return fmt.Errorf("instance %q not found - run upload first to create it", instanceName)
	}

	slog.Info("using existing instance",
		"name", instanceName,
		"seed", instance.Seed,
	)

	// Create retrieve burst runner
	retrieveRunner := runner.NewRetrieveBurstRunner(
		instance.ID,
		cfg.Runner.Retrieve.Burst,
		s,
		guppyClient,
		metrics,
		cfg.Guppy.Email,
	)

	// Run the stress test
	return retrieveRunner.Run(ctx)
}

// RunUploadContinuousApp creates and runs the continuous upload stress test
func RunUploadContinuousApp(ctx context.Context, cfg *config.Config, instanceName string, seedOverride int64) error {
	// Create store
	s, err := store.NewGORMStore(store.StoreConfig{
		Type: cfg.Store.Type,
		Path: cfg.Store.Path,
		DSN:  cfg.Store.DSN,
	})
	if err != nil {
		return fmt.Errorf("failed to create store: %w", err)
	}
	defer s.Close()

	// Create telemetry provider
	provider, err := telemetry.NewProvider(telemetry.Config{
		ServiceName:    cfg.Telemetry.ServiceName,
		PrometheusPort: cfg.Telemetry.PrometheusPort,
		OTLPEndpoint:   cfg.Telemetry.OTLPEndpoint,
	})
	if err != nil {
		return fmt.Errorf("failed to create telemetry provider: %w", err)
	}

	if err := provider.Start(); err != nil {
		return fmt.Errorf("failed to start telemetry: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := provider.Stop(shutdownCtx); err != nil {
			slog.Warn("telemetry shutdown error", "error", err)
		}
	}()

	// Create metrics
	metrics, err := telemetry.NewMetrics(provider)
	if err != nil {
		return fmt.Errorf("failed to create metrics: %w", err)
	}

	// Create guppy client
	guppyClient := guppy.NewCLIClient(
		cfg.Guppy.BinaryPath,
		cfg.Guppy.ConfigPath,
		cfg.Guppy.Email,
	)

	// Get or create instance
	seed := seedOverride
	if seed == 0 {
		seed = cfg.Generator.Seed
	}
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	instance, created, err := s.GetOrCreateInstance(ctx, instanceName, seed)
	if err != nil {
		return fmt.Errorf("failed to get or create instance: %w", err)
	}

	if created {
		slog.Info("created new instance", "name", instanceName, "seed", instance.Seed)
	} else {
		slog.Info("joined existing instance", "name", instanceName, "seed", instance.Seed)
	}

	// Check that spaces exist (continuous mode requires existing spaces)
	spaceCount, err := s.GetSpaceCount(ctx, instance.ID)
	if err != nil {
		return fmt.Errorf("failed to get space count: %w", err)
	}
	if spaceCount == 0 {
		return fmt.Errorf("no spaces found for instance %q - run 'stress upload burst' first to create spaces", instanceName)
	}

	slog.Info("found spaces for continuous upload", "count", spaceCount)

	// Create generator with instance's seed
	gen, err := generator.NewGenerator(generator.Config{
		Seed:        instance.Seed,
		MinFileSize: cfg.Generator.MinFileSize,
		MaxFileSize: cfg.Generator.MaxFileSize,
		BaseDir:     "/tmp",
	})
	if err != nil {
		return fmt.Errorf("failed to create generator: %w", err)
	}

	// Create upload continuous runner
	uploadRunner, err := runner.NewUploadContinuousRunner(
		instance.ID,
		cfg.Runner.Upload.Continuous,
		s,
		guppyClient,
		metrics,
		gen,
		cfg.Guppy.Email,
	)
	if err != nil {
		return fmt.Errorf("failed to create upload runner: %w", err)
	}

	// Run the stress test
	return uploadRunner.Run(ctx)
}

// RunRetrieveContinuousApp creates and runs the continuous retrieve stress test
func RunRetrieveContinuousApp(ctx context.Context, cfg *config.Config, instanceName string) error {
	// Create store
	s, err := store.NewGORMStore(store.StoreConfig{
		Type: cfg.Store.Type,
		Path: cfg.Store.Path,
		DSN:  cfg.Store.DSN,
	})
	if err != nil {
		return fmt.Errorf("failed to create store: %w", err)
	}
	defer s.Close()

	// Create telemetry provider
	provider, err := telemetry.NewProvider(telemetry.Config{
		ServiceName:    cfg.Telemetry.ServiceName,
		PrometheusPort: cfg.Telemetry.PrometheusPort,
		OTLPEndpoint:   cfg.Telemetry.OTLPEndpoint,
	})
	if err != nil {
		return fmt.Errorf("failed to create telemetry provider: %w", err)
	}

	if err := provider.Start(); err != nil {
		return fmt.Errorf("failed to start telemetry: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := provider.Stop(shutdownCtx); err != nil {
			slog.Warn("telemetry shutdown error", "error", err)
		}
	}()

	// Create metrics
	metrics, err := telemetry.NewMetrics(provider)
	if err != nil {
		return fmt.Errorf("failed to create metrics: %w", err)
	}

	// Create guppy client
	guppyClient := guppy.NewCLIClient(
		cfg.Guppy.BinaryPath,
		cfg.Guppy.ConfigPath,
		cfg.Guppy.Email,
	)

	// Get instance (must exist for retrieval)
	instance, err := s.GetInstance(ctx, instanceName)
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}
	if instance == nil {
		return fmt.Errorf("instance %q not found - run upload first to create it", instanceName)
	}

	// Check that uploads exist
	uploadCount, err := s.GetUploadCount(ctx, instance.ID)
	if err != nil {
		return fmt.Errorf("failed to get upload count: %w", err)
	}
	if uploadCount == 0 {
		return fmt.Errorf("no uploads found for instance %q - run 'stress upload burst' first", instanceName)
	}

	slog.Info("using existing instance",
		"name", instanceName,
		"seed", instance.Seed,
		"uploads", uploadCount,
	)

	// Create retrieve continuous runner
	retrieveRunner, err := runner.NewRetrieveContinuousRunner(
		instance.ID,
		cfg.Runner.Retrieve.Continuous,
		s,
		guppyClient,
		metrics,
		cfg.Guppy.Email,
	)
	if err != nil {
		return fmt.Errorf("failed to create retrieve runner: %w", err)
	}

	// Run the stress test
	return retrieveRunner.Run(ctx)
}
