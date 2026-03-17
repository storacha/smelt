package retrieve

import (
	"github.com/spf13/cobra"
)

// Cmd is the parent command for retrieve operations
var Cmd = &cobra.Command{
	Use:   "retrieve",
	Short: "Retrieval stress testing commands",
	Long:  `Commands for running retrieval stress tests against the Storacha network.`,
}

func init() {
	Cmd.AddCommand(burstCmd)
	Cmd.AddCommand(continuousCmd)
	Cmd.AddCommand(statusCmd)
}
