package generate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/storacha/smelt/pkg/manifest"
)

// Options controls the generation process.
type Options struct {
	// ManifestPath is an optional explicit manifest override. When empty
	// Generate falls back to manifest.ResolveManifestPath which picks the
	// session manifest (from a prior snapshot load) when present and the
	// project manifest otherwise.
	ManifestPath string
	ProjectDir   string // Project root directory
	Force        bool   // Overwrite existing keys
}

// Result contains the paths to all generated artifacts.
type Result struct {
	PiriComposePath string
	KeysDir         string
	NodeCount       int
	// ManifestPath is the path Generate actually read from.
	ManifestPath string
}

// Generate reads the manifest, generates keys, and produces Docker Compose files.
func Generate(opts Options) (*Result, error) {
	manifestPath := opts.ManifestPath
	fromSession := false
	if manifestPath == "" {
		manifestPath, fromSession = manifest.ResolveManifestPath(opts.ProjectDir)
	}
	if fromSession {
		fmt.Printf("Using session manifest: %s\n", manifestPath)
	}

	m, err := manifest.Parse(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	nodes, err := m.Resolve()
	if err != nil {
		return nil, fmt.Errorf("resolve manifest: %w", err)
	}

	keysDir := filepath.Join(opts.ProjectDir, "generated", "keys")
	composeDir := filepath.Join(opts.ProjectDir, "generated", "compose")

	if err := os.MkdirAll(composeDir, 0755); err != nil {
		return nil, fmt.Errorf("create compose dir: %w", err)
	}

	// Generate keys.
	if err := GenerateKeys(keysDir, nodes, opts.Force); err != nil {
		return nil, fmt.Errorf("generate keys: %w", err)
	}

	// Generate piri compose.
	piriYAML, err := GeneratePiriCompose(nodes)
	if err != nil {
		return nil, fmt.Errorf("generate piri compose: %w", err)
	}
	piriPath := filepath.Join(composeDir, "piri.yml")
	if err := os.WriteFile(piriPath, piriYAML, 0644); err != nil {
		return nil, fmt.Errorf("write piri compose: %w", err)
	}

	return &Result{
		PiriComposePath: piriPath,
		KeysDir:         keysDir,
		NodeCount:       len(nodes),
		ManifestPath:    manifestPath,
	}, nil
}
