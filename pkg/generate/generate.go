package generate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/storacha/smelt/pkg/manifest"
)

// Options controls the generation process.
type Options struct {
	ManifestPath string // Path to smelt.yml
	ProjectDir   string // Project root directory
	Force        bool   // Overwrite existing keys
}

// Result contains the paths to all generated artifacts.
type Result struct {
	PiriComposePath      string
	PiriPortsComposePath string
	KeysDir              string
	NodeCount            int
}

// Generate reads the manifest, generates keys, and produces Docker Compose files.
func Generate(opts Options) (*Result, error) {
	m, err := manifest.Parse(opts.ManifestPath)
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

	// Generate piri ports compose (separate file for optional port mappings).
	piriPortsYAML, err := GeneratePiriPortsCompose(nodes)
	if err != nil {
		return nil, fmt.Errorf("generate piri ports compose: %w", err)
	}
	piriPortsPath := filepath.Join(composeDir, "piri.ports.yml")
	if err := os.WriteFile(piriPortsPath, piriPortsYAML, 0644); err != nil {
		return nil, fmt.Errorf("write piri ports compose: %w", err)
	}

	return &Result{
		PiriComposePath:      piriPath,
		PiriPortsComposePath: piriPortsPath,
		KeysDir:              keysDir,
		NodeCount:            len(nodes),
	}, nil
}
