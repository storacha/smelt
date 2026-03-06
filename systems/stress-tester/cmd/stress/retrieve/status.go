package retrieve

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/storacha/smelt/systems/stress-tester/internal/config"
	"github.com/storacha/smelt/systems/stress-tester/internal/store"
)

// formatBytes converts bytes to human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show retrieval statistics",
	Long: `Display retrieval statistics from the stress test database.

If --instance is provided, shows stats for that specific instance.
Otherwise, shows stats for all instances.`,
	RunE: showRetrieveStatus,
}

func init() {
	statusCmd.Flags().String("instance", "", "instance name (optional, shows all if omitted)")
}

func showRetrieveStatus(cmd *cobra.Command, args []string) error {
	instanceName, _ := cmd.Flags().GetString("instance")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Open store
	s, err := store.NewGORMStore(store.StoreConfig{
		Type: cfg.Store.Type,
		Path: cfg.Store.Path,
		DSN:  cfg.Store.DSN,
	})
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}
	defer s.Close()

	ctx := context.Background()

	fmt.Println("=== Retrieval Status ===")
	fmt.Println()

	if instanceName != "" {
		// Show stats for specific instance
		return showInstanceRetrieveStatus(ctx, s, instanceName)
	}

	// Show stats for all instances
	instances, err := s.ListInstances(ctx)
	if err != nil {
		return fmt.Errorf("failed to list instances: %w", err)
	}

	if len(instances) == 0 {
		fmt.Println("No instances found.")
		return nil
	}

	for _, inst := range instances {
		fmt.Printf("--- Instance: %s ---\n", inst.Name)
		fmt.Printf("Seed: %d\n", inst.Seed)
		fmt.Printf("Created: %s\n", inst.CreatedAt.Format("2006-01-02 15:04:05"))

		stats, err := s.GetRetrievalStats(ctx, inst.ID)
		if err != nil {
			return fmt.Errorf("failed to get retrieval stats: %w", err)
		}

		if stats.TotalRetrievals > 0 {
			fmt.Printf("Total retrievals: %d (%s)\n", stats.TotalRetrievals, formatBytes(stats.TotalBytes))
			fmt.Printf("Success rate: %.1f%%\n", stats.SuccessRate*100)
			fmt.Printf("Avg latency: %.2fms\n", stats.AvgDurationMs)
		} else {
			fmt.Printf("Total retrievals: 0\n")
		}
		fmt.Println()
	}

	return nil
}

func showInstanceRetrieveStatus(ctx context.Context, s store.Store, instanceName string) error {
	instance, err := s.GetInstance(ctx, instanceName)
	if err != nil {
		return fmt.Errorf("failed to get instance: %w", err)
	}
	if instance == nil {
		return fmt.Errorf("instance %q not found", instanceName)
	}

	fmt.Printf("Instance: %s\n", instance.Name)
	fmt.Printf("Seed: %d\n", instance.Seed)
	fmt.Printf("Created: %s\n", instance.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Println()

	// Get retrieval stats
	stats, err := s.GetRetrievalStats(ctx, instance.ID)
	if err != nil {
		return fmt.Errorf("failed to get retrieval stats: %w", err)
	}

	fmt.Println("--- Retrievals ---")
	if stats.TotalRetrievals > 0 {
		fmt.Printf("Total retrievals: %d (%s)\n", stats.TotalRetrievals, formatBytes(stats.TotalBytes))
	} else {
		fmt.Printf("Total retrievals: 0\n")
	}
	fmt.Printf("Successful: %d\n", stats.SuccessCount)
	fmt.Printf("Failed: %d\n", stats.FailureCount)

	if stats.TotalRetrievals > 0 {
		fmt.Printf("Success rate: %.1f%%\n", stats.SuccessRate*100)
		fmt.Println()
		fmt.Println("--- Latency ---")
		fmt.Printf("Average: %.2fms\n", stats.AvgDurationMs)
		fmt.Printf("P50: %.2fms\n", stats.P50DurationMs)
		fmt.Printf("P95: %.2fms\n", stats.P95DurationMs)
		fmt.Printf("P99: %.2fms\n", stats.P99DurationMs)
	}

	if stats.LastRetrievalTime != nil {
		fmt.Println()
		fmt.Printf("Last retrieval: %s\n", stats.LastRetrievalTime.Format("2006-01-02 15:04:05"))
	}

	return nil
}
