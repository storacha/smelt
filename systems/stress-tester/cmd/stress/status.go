package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/storacha/smelt/systems/stress-tester/internal/config"
	"github.com/storacha/smelt/systems/stress-tester/internal/store"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "List all stress test instances",
	Long: `Display all stress test instances and their summary statistics.

For detailed upload statistics, use: stress upload status --instance <name>
For detailed retrieval statistics, use: stress retrieve status --instance <name>`,
	RunE: showStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

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

func showStatus(cmd *cobra.Command, args []string) error {
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

	// List all instances
	instances, err := s.ListInstances(ctx)
	if err != nil {
		return fmt.Errorf("failed to list instances: %w", err)
	}

	fmt.Println("=== Stress Test Instances ===")
	fmt.Println()
	fmt.Printf("Database: %s (%s)\n", cfg.Store.Path, cfg.Store.Type)
	fmt.Println()

	if len(instances) == 0 {
		fmt.Println("No instances found.")
		fmt.Println()
		fmt.Println("Create one with: stress upload burst --instance <name>")
		return nil
	}

	fmt.Printf("Found %d instance(s):\n\n", len(instances))

	for _, inst := range instances {
		spaceCount, _ := s.GetSpaceCount(ctx, inst.ID)
		uploadStats, _ := s.GetUploadStats(ctx, inst.ID)
		retrievalStats, _ := s.GetRetrievalStats(ctx, inst.ID)

		fmt.Printf("--- %s ---\n", inst.Name)
		fmt.Printf("  Seed:       %d\n", inst.Seed)
		fmt.Printf("  Created:    %s\n", inst.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Spaces:     %d\n", spaceCount)

		if uploadStats != nil && uploadStats.TotalUploads > 0 {
			fmt.Printf("  Uploads:    %d (%s)\n", uploadStats.TotalUploads, formatBytes(uploadStats.TotalBytes))
		} else {
			fmt.Printf("  Uploads:    0\n")
		}

		if retrievalStats != nil && retrievalStats.TotalRetrievals > 0 {
			fmt.Printf("  Retrievals: %d (%s, %.1f%% success)\n",
				retrievalStats.TotalRetrievals, formatBytes(retrievalStats.TotalBytes), retrievalStats.SuccessRate*100)
		} else {
			fmt.Printf("  Retrievals: 0\n")
		}
		fmt.Println()
	}

	return nil
}
