package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/storacha/smelt/pkg/snapshot"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Capture and restore smelt stack state",
	Long: `Snapshot a running smelt stack (contract state, keys, volumes, manifest)
into a named directory under generated/snapshots/, then restore it later to
skip the slow on-chain registration step during development.`,
}

var snapshotSaveCmd = &cobra.Command{
	Use:   "save NAME",
	Short: "Save the running stack to a named snapshot",
	Long: `Stops the stack gracefully, captures the in-memory anvil state,
archives every docker volume in the resolved manifest, and copies keys and
smelt.yml. The stack is left stopped on success.`,
	Args: cobra.ExactArgs(1),
	RunE: runSnapshotSave,
}

var snapshotLoadCmd = &cobra.Command{
	Use:   "load NAME_OR_PATH",
	Short: "Restore a snapshot into the current project state",
	Long: `Populates keys, proofs, blockchain state, and docker volumes from
the snapshot. The snapshot's smelt.yml is installed as a session manifest
(at generated/snapshot-scratch/smelt.yml) so subsequent generate/save calls
drive off it — the project root smelt.yml is never modified.

Accepts either a snapshot name (resolved under generated/snapshots/) or a
path to a snapshot directory elsewhere on disk. Stack must be fully stopped.
Run 'make up' after to boot from the restored state, or use
'make up SNAPSHOT=<name-or-path>' to do both in one step.`,
	Args: cobra.ExactArgs(1),
	RunE: runSnapshotLoad,
}

var snapshotListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available snapshots",
	RunE:  runSnapshotList,
}

var snapshotRmCmd = &cobra.Command{
	Use:   "rm NAME",
	Short: "Delete a snapshot",
	Args:  cobra.ExactArgs(1),
	RunE:  runSnapshotRm,
}

func init() {
	rootCmd.AddCommand(snapshotCmd)
	snapshotCmd.AddCommand(snapshotSaveCmd)
	snapshotCmd.AddCommand(snapshotLoadCmd)
	snapshotCmd.AddCommand(snapshotListCmd)
	snapshotCmd.AddCommand(snapshotRmCmd)

	snapshotSaveCmd.Flags().StringP("project-dir", "d", ".", "project root directory")
	snapshotSaveCmd.Flags().Bool("force", false, "overwrite an existing snapshot with the same name")

	snapshotLoadCmd.Flags().StringP("project-dir", "d", ".", "project root directory")

	snapshotListCmd.Flags().StringP("project-dir", "d", ".", "project root directory")

	snapshotRmCmd.Flags().StringP("project-dir", "d", ".", "project root directory")
}

func runSnapshotSave(cmd *cobra.Command, args []string) error {
	projectDir, _ := cmd.Flags().GetString("project-dir")
	force, _ := cmd.Flags().GetBool("force")
	return snapshot.Save(cmd.Context(), snapshot.SaveOpts{
		ProjectDir: projectDir,
		Name:       args[0],
		Force:      force,
	})
}

func runSnapshotLoad(cmd *cobra.Command, args []string) error {
	projectDir, _ := cmd.Flags().GetString("project-dir")
	return snapshot.Load(cmd.Context(), snapshot.LoadOpts{
		ProjectDir: projectDir,
		NameOrPath: args[0],
	})
}

func runSnapshotList(cmd *cobra.Command, args []string) error {
	projectDir, _ := cmd.Flags().GetString("project-dir")
	snaps, err := snapshot.List(projectDir)
	if err != nil {
		return err
	}
	if len(snaps) == 0 {
		fmt.Println("no snapshots")
		return nil
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tCREATED\tSIZE\tVOLUMES")
	for _, s := range snaps {
		age := "-"
		if !s.CreatedAt.IsZero() {
			age = humanAge(s.CreatedAt)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", s.Name, age, humanSize(s.SizeBytes), len(s.Volumes))
	}
	return tw.Flush()
}

func runSnapshotRm(cmd *cobra.Command, args []string) error {
	projectDir, _ := cmd.Flags().GetString("project-dir")
	return snapshot.Remove(projectDir, args[0])
}

func humanAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
