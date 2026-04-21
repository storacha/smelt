package stack

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/storacha/smelt"
)

// embeddedSnapshotsRoot is the path within smelt.EmbeddedFiles under
// which curated snapshots live. Mirrors the on-disk layout at the repo
// root (snapshots/<name>/...).
const embeddedSnapshotsRoot = "snapshots"

// ListEmbeddedSnapshots returns the names of snapshots bundled with
// this smelt module. Each name is usable as the argument to
// WithEmbeddedSnapshot.
//
// External consumers (packages that import smelt) should call this
// rather than hunting for snapshot paths on disk — paths into the
// smelt repo don't exist in a consumer's checkout.
func ListEmbeddedSnapshots() ([]string, error) {
	entries, err := fs.ReadDir(smelt.EmbeddedFiles, embeddedSnapshotsRoot)
	if err != nil {
		// When the snapshots/ dir is empty at build time, go:embed still
		// creates an empty FS — propagate the actual error so callers
		// know the difference between "nothing embedded" and "embed failed".
		return nil, fmt.Errorf("list embedded snapshots: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// extractEmbeddedSnapshot materializes the named snapshot from the
// embedded FS into a sibling dir under parent and returns its absolute
// path. The result is suitable as the `path` argument to the normal
// snapshot-load flow — by re-using an on-disk path here we avoid
// forking the load logic for embedded vs. external snapshots.
func extractEmbeddedSnapshot(name, parent string) (string, error) {
	srcRoot := embeddedSnapshotsRoot + "/" + name
	if _, err := fs.Stat(smelt.EmbeddedFiles, srcRoot); err != nil {
		available, _ := ListEmbeddedSnapshots()
		return "", fmt.Errorf("embedded snapshot %q not found (available: %v)", name, available)
	}

	dstRoot := filepath.Join(parent, "embedded-snapshot-"+name)
	if err := os.MkdirAll(dstRoot, 0755); err != nil {
		return "", err
	}

	err := fs.WalkDir(smelt.EmbeddedFiles, srcRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(srcRoot, path)
		if relErr != nil {
			return relErr
		}
		dst := filepath.Join(dstRoot, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0755)
		}
		data, err := smelt.EmbeddedFiles.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0644)
	})
	if err != nil {
		return "", fmt.Errorf("extract embedded snapshot %q: %w", name, err)
	}
	return dstRoot, nil
}
