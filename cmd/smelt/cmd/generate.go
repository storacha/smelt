package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	generateCmd.Flags().StringP("manifest", "m", "smelt.yml", "path to manifest file (env SMELT_MANIFEST)")
	generateCmd.Flags().StringP("project-dir", "d", ".", "project root directory (env SMELT_PROJECT_DIR)")
	generateCmd.Flags().Bool("force", false, "overwrite existing keys (env SMELT_FORCE)")

	viper.BindPFlag("manifest", generateCmd.Flags().Lookup("manifest"))
	viper.BindPFlag("project-dir", generateCmd.Flags().Lookup("project-dir"))
	viper.BindPFlag("force", generateCmd.Flags().Lookup("force"))
}

func runGenerate(cmd *cobra.Command, args []string) error {
	result, err := generate.Generate(generate.Options{
		ManifestPath: viper.GetString("manifest"),
		ProjectDir:   viper.GetString("project-dir"),
		Force:        viper.GetBool("force"),
	})
	if err != nil {
		return err
	}

	fmt.Printf("Generated %d piri node(s)\n", result.NodeCount)
	fmt.Printf("  Compose: %s\n", result.PiriComposePath)
	fmt.Printf("  Keys: %s\n", result.KeysDir)
	return nil
}
