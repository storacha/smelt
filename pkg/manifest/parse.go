package manifest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

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
