package upload

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
	Short: "Show upload statistics",
	Long: `Display upload statistics from the stress test database.

If --instance is provided, shows stats for that specific instance.
Otherwise, shows stats for all instances.`,
	RunE: showUploadStatus,
}

func init() {
	statusCmd.Flags().String("instance", "", "instance name (optional, shows all if omitted)")
}

func showUploadStatus(cmd *cobra.Command, args []string) error {
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

	fmt.Println("=== Upload Status ===")
	fmt.Println()

	if instanceName != "" {
		// Show stats for specific instance
		return showInstanceUploadStatus(ctx, s, instanceName)
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

		spaceCount, err := s.GetSpaceCount(ctx, inst.ID)
		if err != nil {
			return fmt.Errorf("failed to get space count: %w", err)
		}
		fmt.Printf("Spaces: %d\n", spaceCount)

		uploadStats, err := s.GetUploadStats(ctx, inst.ID)
		if err != nil {
			return fmt.Errorf("failed to get upload stats: %w", err)
		}
		if uploadStats != nil && uploadStats.TotalUploads > 0 {
			fmt.Printf("Uploads: %d (%s)\n", uploadStats.TotalUploads, formatBytes(uploadStats.TotalBytes))
		} else {
			fmt.Printf("Uploads: 0\n")
		}
		fmt.Println()
	}

	return nil
}

func showInstanceUploadStatus(ctx context.Context, s store.Store, instanceName string) error {
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

	spaceCount, err := s.GetSpaceCount(ctx, instance.ID)
	if err != nil {
		return fmt.Errorf("failed to get space count: %w", err)
	}
	fmt.Printf("Total spaces: %d\n", spaceCount)

	uploadStats, err := s.GetUploadStats(ctx, instance.ID)
	if err != nil {
		return fmt.Errorf("failed to get upload stats: %w", err)
	}
	if uploadStats != nil && uploadStats.TotalUploads > 0 {
		fmt.Printf("Total uploads: %d (%s)\n", uploadStats.TotalUploads, formatBytes(uploadStats.TotalBytes))
	} else {
		fmt.Printf("Total uploads: 0\n")
	}

	// Show spaces with upload counts
	spaces, err := s.GetSpaces(ctx, instance.ID)
	if err != nil {
		return fmt.Errorf("failed to get spaces: %w", err)
	}

	if len(spaces) > 0 {
		fmt.Println()
		fmt.Println("Spaces:")
		for _, space := range spaces {
			uploads, err := s.GetUploadsForSpace(ctx, space.DID)
			if err != nil {
				return fmt.Errorf("failed to get uploads for space: %w", err)
			}
			var totalBytes int64
			for _, u := range uploads {
				totalBytes += u.SizeBytes
			}
			fmt.Printf("  %s: %d uploads, %d bytes\n", space.DID, len(uploads), totalBytes)
		}
	}

	return nil
}
