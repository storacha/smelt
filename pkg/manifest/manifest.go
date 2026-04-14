// Package manifest defines the smelt.yml schema and resolves it into
// concrete node configurations ready for compose generation.
package manifest

import "fmt"

const (
	// MaxPiriNodes is limited by the number of Anvil pre-funded accounts.
	// Account 0 = piri-0 wallet, account 1 = payer (signing-service),
	// accounts 2-9 = piri-1 through piri-8.
	MaxPiriNodes = 9

	DBSQLite     = "sqlite"
	DBPostgres   = "postgres"
	BlobFS       = "filesystem"
	BlobS3       = "s3"
)

// Manifest is the top-level smelt.yml schema.
type Manifest struct {
	Version int      `yaml:"version"`
	Piri    PiriSpec `yaml:"piri"`
}

// PiriSpec describes the desired piri node topology.
// Use either Count (shorthand for N identical nodes) or Nodes (explicit per-node config).
type PiriSpec struct {
	Count    int            `yaml:"count,omitempty"`
	Defaults PiriDefaults   `yaml:"defaults,omitempty"`
	Nodes    []PiriNodeSpec `yaml:"nodes,omitempty"`
}

// PiriDefaults are inherited by every node unless overridden.
type PiriDefaults struct {
	Image   string      `yaml:"image,omitempty"`
	Storage StorageSpec `yaml:"storage,omitempty"`
}

// PiriNodeSpec describes a single piri node.
type PiriNodeSpec struct {
	Name    string      `yaml:"name,omitempty"`
	Image   string      `yaml:"image,omitempty"`
	Storage StorageSpec `yaml:"storage,omitempty"`
}

// StorageSpec controls piri's database and blob backends.
type StorageSpec struct {
	DB   string `yaml:"db,omitempty"`
	Blob string `yaml:"blob,omitempty"`
}

// ResolvedPiriNode is a fully resolved node ready for compose generation.
type ResolvedPiriNode struct {
	Name    string
	Index   int
	Image   string
	Storage StorageSpec
}

// Resolve normalizes the manifest into a concrete list of resolved nodes.
func (m *Manifest) Resolve() ([]ResolvedPiriNode, error) {
	spec := &m.Piri

	if spec.Count > 0 && len(spec.Nodes) > 0 {
		return nil, fmt.Errorf("manifest: cannot specify both 'count' and 'nodes'")
	}

	// Expand count shorthand into explicit nodes.
	var nodes []PiriNodeSpec
	if spec.Count > 0 {
		nodes = make([]PiriNodeSpec, spec.Count)
	} else if len(spec.Nodes) > 0 {
		nodes = spec.Nodes
	} else {
		// Default: single node.
		nodes = []PiriNodeSpec{{}}
	}

	if len(nodes) > MaxPiriNodes {
		return nil, fmt.Errorf("manifest: %d piri nodes exceeds maximum of %d (limited by Anvil accounts)", len(nodes), MaxPiriNodes)
	}

	// Apply defaults and auto-generate names.
	resolved := make([]ResolvedPiriNode, len(nodes))
	seen := make(map[string]bool)

	for i, n := range nodes {
		r := ResolvedPiriNode{Index: i}

		// Name
		if n.Name != "" {
			r.Name = n.Name
		} else {
			r.Name = fmt.Sprintf("piri-%d", i)
		}
		if seen[r.Name] {
			return nil, fmt.Errorf("manifest: duplicate node name %q", r.Name)
		}
		seen[r.Name] = true

		// Image: node override > defaults > empty (uses PIRI_IMAGE env var at runtime)
		r.Image = firstNonEmpty(n.Image, spec.Defaults.Image)

		// Storage: node override > defaults > hardcoded defaults
		r.Storage.DB = firstNonEmpty(n.Storage.DB, spec.Defaults.Storage.DB, DBSQLite)
		r.Storage.Blob = firstNonEmpty(n.Storage.Blob, spec.Defaults.Storage.Blob, BlobFS)

		if err := validateStorage(r.Storage); err != nil {
			return nil, fmt.Errorf("manifest: node %q: %w", r.Name, err)
		}

		resolved[i] = r
	}

	return resolved, nil
}

func validateStorage(s StorageSpec) error {
	switch s.DB {
	case DBSQLite, DBPostgres:
	default:
		return fmt.Errorf("invalid db backend %q (must be %q or %q)", s.DB, DBSQLite, DBPostgres)
	}
	switch s.Blob {
	case BlobFS, BlobS3:
	default:
		return fmt.Errorf("invalid blob backend %q (must be %q or %q)", s.Blob, BlobFS, BlobS3)
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
