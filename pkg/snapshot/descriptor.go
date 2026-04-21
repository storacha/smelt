package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DescriptorFile is the filename of the snapshot manifest that lives at the
// root of every snapshot directory.
const DescriptorFile = "manifest.json"

// Descriptor records what a snapshot contains at the moment it was taken.
// It's provenance, not configuration: loading a snapshot reads the descriptor
// to know which volume archives to restore, not to decide what to include.
type Descriptor struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	// Volumes lists the docker-compose declared names (e.g. "piri-0-data"),
	// NOT the project-namespaced names. The restore path reapplies the
	// current project name.
	Volumes []string `json:"volumes"`
	// Keys lists filenames copied from generated/keys/ (e.g. "piri-0.pem").
	Keys []string `json:"keys"`
	// Proofs lists filenames copied from generated/proofs/ (e.g.
	// "piri-0-proof.txt"). These are UCAN delegations that upload's
	// post_start.sh uses to register piri as a storage provider.
	Proofs []string `json:"proofs"`
	// Images records the per-service image identity at save time: both the
	// human-readable reference (tag) and the immutable digest. Load
	// compares both and warns independently on tag drift (teammate's `.env`
	// disagrees) and digest drift at the same tag (someone re-pulled a
	// moving tag between save and load).
	Images map[string]ImageInfo `json:"images,omitempty"`
}

// ImageInfo is the per-service image identity captured in a snapshot.
type ImageInfo struct {
	// Tag is the resolved image reference from `docker compose config`,
	// e.g. "ghcr.io/storacha/piri:main" or "filecoin-localdev:local".
	Tag string `json:"tag"`
	// Digest is the immutable content identifier: a RepoDigest
	// ("ghcr.io/storacha/piri@sha256:…") when the image came from a
	// registry, or the docker image Id ("sha256:…") for locally-built
	// images that have no RepoDigest yet.
	Digest string `json:"digest,omitempty"`
}

func writeDescriptor(dir string, d *Descriptor) error {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal descriptor: %w", err)
	}
	path := filepath.Join(dir, DescriptorFile)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write descriptor: %w", err)
	}
	return nil
}

func readDescriptor(dir string) (*Descriptor, error) {
	path := filepath.Join(dir, DescriptorFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read descriptor: %w", err)
	}
	var d Descriptor
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("parse descriptor: %w", err)
	}
	return &d, nil
}
