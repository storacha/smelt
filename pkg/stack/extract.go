package stack

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/storacha/smelt"
)

// extractFiles extracts all embedded files to a test temp directory,
// maintaining the exact directory structure required for compose.
func extractFiles(t *testing.T) (string, error) {
	return extractFilesToDir(t.TempDir())
}

// extractFilesToDir extracts all embedded files to the given directory.
func extractFilesToDir(dir string) (string, error) {
	// Walk the embedded filesystem and copy all files
	err := fs.WalkDir(smelt.EmbeddedFiles, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		destPath := filepath.Join(dir, path)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		// Read the embedded file
		data, err := smelt.EmbeddedFiles.ReadFile(path)
		if err != nil {
			return err
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		// Write the file, preserving executable bit for shell scripts
		perm := os.FileMode(0644)
		if filepath.Ext(path) == ".sh" {
			perm = 0755
		}

		return os.WriteFile(destPath, data, perm)
	})
	if err != nil {
		return "", err
	}

	// Create the generated directory structure for keys and proofs
	generatedDirs := []string{
		filepath.Join(dir, "generated", "keys"),
		filepath.Join(dir, "generated", "proofs"),
	}
	for _, d := range generatedDirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return "", err
		}
	}

	// TODO stress tester is a wip
	// Create stub for stress-tester (in separate Go module, can't embed)
	/*
			stressDir := filepath.Join(tempDir, "systems", "stress-tester")
			if err := os.MkdirAll(stressDir, 0755); err != nil {
				return "", err
			}
			stressCompose := `# Stub - stress-tester not available in smeltery
		services: {}
		`
			if err := os.WriteFile(filepath.Join(stressDir, "compose.yml"), []byte(stressCompose), 0644); err != nil {
				return "", err
			}
	*/

	return dir, nil
}
