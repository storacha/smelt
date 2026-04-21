package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/storacha/smelt/pkg/generate"
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate Docker Compose files and keys from manifest",
	Long: `Reads smelt.yml and generates:
  - Docker Compose configuration for N piri nodes
  - Ed25519 identity keys for all services
  - EVM wallet keys for each piri node`,
	RunE: runGenerate,
}

func init() {
	rootCmd.AddCommand(generateCmd)
	// Default is empty so Generate's session-aware resolver picks the right
	// manifest: generated/snapshot-scratch/smelt.yml during a snapshot
	// session, smelt.yml otherwise. Pass an explicit path to force.
	generateCmd.Flags().StringP("manifest", "m", "", "path to manifest file (default: auto-detect)")
	generateCmd.Flags().StringP("project-dir", "d", ".", "project root directory")
	generateCmd.Flags().Bool("force", false, "overwrite existing keys")
}

func runGenerate(cmd *cobra.Command, args []string) error {
	manifestPath, _ := cmd.Flags().GetString("manifest")
	projectDir, _ := cmd.Flags().GetString("project-dir")
	force, _ := cmd.Flags().GetBool("force")

	result, err := generate.Generate(generate.Options{
		ManifestPath: manifestPath,
		ProjectDir:   projectDir,
		Force:        force,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Generated %d piri node(s)\n", result.NodeCount)
	fmt.Printf("  Compose: %s\n", result.PiriComposePath)
	fmt.Printf("  Keys: %s\n", result.KeysDir)
	return nil
}
