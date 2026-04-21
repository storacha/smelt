package stack

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/storacha/smelt/pkg/manifest"
	"github.com/storacha/smelt/pkg/snapshot"
)

// resolveSnapshotDir resolves the user-provided snapshot path to an
// absolute directory, verifying it contains a snapshot descriptor.
// Relative paths resolve against the caller's working directory.
func resolveSnapshotDir(path string) (string, error) {
	if path == "" {
		return "", errors.New("snapshot path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve snapshot path: %w", err)
	}
	if _, err := os.Stat(abs); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("snapshot not found at %s", abs)
		}
		return "", err
	}
	if _, err := os.Stat(filepath.Join(abs, snapshot.DescriptorFile)); err != nil {
		return "", fmt.Errorf("not a valid snapshot dir (missing %s): %s",
			snapshot.DescriptorFile, abs)
	}
	return abs, nil
}

// validateSnapshotOptions rejects option combinations that collide with
// a snapshot's embedded topology. Other options (images, timeout,
// keepOnFailure, piriBinaryPath) are compatible and pass through.
func validateSnapshotOptions(cfg *config) error {
	if cfg.piriNodes != nil {
		return errors.New("WithSnapshot is incompatible with WithPiriCount / WithPiriNodes " +
			"(topology is sourced from the snapshot's smelt.yml)")
	}
	return nil
}

// loadSnapshotTopology parses the snapshot's embedded smelt.yml and
// returns the resolved piri-node list the stack should stand up.
func loadSnapshotTopology(snapshotDir string) ([]manifest.ResolvedPiriNode, error) {
	m, err := manifest.Parse(filepath.Join(snapshotDir, "smelt.yml"))
	if err != nil {
		return nil, fmt.Errorf("parse snapshot smelt.yml: %w", err)
	}
	nodes, err := m.Resolve()
	if err != nil {
		return nil, fmt.Errorf("resolve snapshot manifest: %w", err)
	}
	return nodes, nil
}

// seedBaselineState populates the tempDir's generated/snapshot-scratch/
// with the embedded post-deploy blockchain baseline so the compose
// bind-mounts resolve without docker auto-creating the source paths as
// root-owned directories. Analogous to the Makefile's `ensure-state`
// target; required for pkg/stack's cold-boot path (the snapshot-restore
// path seeds scratch via snapshot.LoadFiles instead).
func seedBaselineState(tempDir string) error {
	scratchDir := filepath.Join(tempDir, "generated", "snapshot-scratch")
	if err := os.MkdirAll(scratchDir, 0755); err != nil {
		return fmt.Errorf("create scratch dir: %w", err)
	}
	for _, f := range []string{"anvil-state.json", "deployed-addresses.json"} {
		src := filepath.Join(tempDir, "systems", "blockchain", "state", f)
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read baseline %s: %w", f, err)
		}
		dst := filepath.Join(scratchDir, f)
		if err := os.WriteFile(dst, data, 0644); err != nil {
			return fmt.Errorf("seed %s: %w", f, err)
		}
	}
	return nil
}
