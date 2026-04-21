package manifest

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SessionManifestPath is the path of the scratch-dir session manifest,
// relative to the project root. A snapshot load writes the loaded
// snapshot's smelt.yml here so subsequent generate/save calls drive off
// the snapshot's topology without touching the tracked project manifest.
const SessionManifestPath = "generated/snapshot-scratch/smelt.yml"

// ProjectManifestPath is the tracked manifest at the project root.
const ProjectManifestPath = "smelt.yml"

// ResolveManifestPath returns the manifest smelt should drive off for the
// given project. If a snapshot session is active (the scratch manifest
// exists), that path is returned; otherwise the tracked project manifest.
// The second return value reports whether the result is the session
// manifest — callers that want to log the choice can use it.
func ResolveManifestPath(projectDir string) (string, bool) {
	session := filepath.Join(projectDir, SessionManifestPath)
	if _, err := os.Stat(session); err == nil {
		return session, true
	}
	return filepath.Join(projectDir, ProjectManifestPath), false
}

// Parse reads a smelt.yml manifest from the given path.
func Parse(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	return ParseBytes(data)
}

// ParseBytes parses a smelt.yml manifest from raw bytes.
func ParseBytes(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}
