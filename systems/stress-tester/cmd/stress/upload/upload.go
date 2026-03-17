package upload

import (
	"github.com/spf13/cobra"
)

// Cmd is the parent command for upload operations
var Cmd = &cobra.Command{
	Use:   "upload",
	Short: "Upload stress testing commands",
	Long:  `Commands for running upload stress tests against the Storacha network.`,
}

func init() {
	Cmd.AddCommand(burstCmd)
	Cmd.AddCommand(continuousCmd)
	Cmd.AddCommand(statusCmd)
}
