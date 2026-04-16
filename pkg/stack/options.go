package stack

import (
	"fmt"
	"time"

	"github.com/storacha/smelt/pkg/manifest"
)

// PiriNodeConfig configures a single piri node in the test stack.
type PiriNodeConfig struct {
	Postgres bool // Use PostgreSQL for this node
	S3       bool // Use S3 for this node
}

// config holds the configuration for a Stack.
type config struct {
	// Image overrides
	piriImage       string
	guppyImage      string
	indexerImage    string
	delegatorImage  string
	uploadImage     string
	signerImage     string
	blockchainImage string
	ipniImage       string

	// Binary overrides (mount local binary instead of using image's binary)
	piriBinaryPath string

	// Piri node topology. When nil, a single default node is used.
	piriNodes []PiriNodeConfig

	// Stack configuration
	timeout       time.Duration
	keepOnFailure bool
}

func defaultConfig() *config {
	return &config{
		timeout: 5 * time.Minute, // Default 5 minute timeout for stack startup
	}
}

// buildEnv returns the environment variables for docker-compose.
func (c *config) buildEnv() map[string]string {
	env := make(map[string]string)

	if c.piriImage != "" {
		env["PIRI_IMAGE"] = c.piriImage
	}
	if c.guppyImage != "" {
		env["GUPPY_IMAGE"] = c.guppyImage
	}
	if c.indexerImage != "" {
		env["INDEXER_IMAGE"] = c.indexerImage
	}
	if c.delegatorImage != "" {
		env["DELEGATOR_IMAGE"] = c.delegatorImage
	}
	if c.uploadImage != "" {
		env["UPLOAD_IMAGE"] = c.uploadImage
	}
	if c.signerImage != "" {
		env["SIGNER_IMAGE"] = c.signerImage
	}
	if c.blockchainImage != "" {
		env["BLOCKCHAIN_IMAGE"] = c.blockchainImage
	}
	if c.ipniImage != "" {
		env["IPNI_IMAGE"] = c.ipniImage
	}

	return env
}

// resolveNodes resolves the piri node configuration into manifest.ResolvedPiriNode list.
func (c *config) resolveNodes() []manifest.ResolvedPiriNode {
	nodes := c.piriNodes
	if nodes == nil {
		nodes = []PiriNodeConfig{{}}
	}

	resolved := make([]manifest.ResolvedPiriNode, len(nodes))
	for i, n := range nodes {
		db := manifest.DBSQLite
		if n.Postgres {
			db = manifest.DBPostgres
		}
		blob := manifest.BlobFS
		if n.S3 {
			blob = manifest.BlobS3
		}
		resolved[i] = manifest.ResolvedPiriNode{
			Name:  fmt.Sprintf("piri-%d", i),
			Index: i,
			Storage: manifest.StorageSpec{
				DB:   db,
				Blob: blob,
			},
		}
	}
	return resolved
}

// Option configures a Stack.
type Option func(*config)

// WithPiriImage sets the piri container image.
func WithPiriImage(image string) Option {
	return func(c *config) {
		c.piriImage = image
	}
}

// WithPiriBinary mounts a local piri binary into the container, replacing the
// image's binary. This enables rapid iteration without rebuilding the container
// image. The binary must be compiled for Linux (use BuildPiriBinary helper).
//
// Example:
//
//	piriBin := stack.BuildPiriBinary(t, "/path/to/piri/repo")
//	s := stack.MustNewStack(t, stack.WithPiriBinary(piriBin))
func WithPiriBinary(path string) Option {
	return func(c *config) {
		c.piriBinaryPath = path
	}
}

// WithGuppyImage sets the guppy container image.
func WithGuppyImage(image string) Option {
	return func(c *config) {
		c.guppyImage = image
	}
}

// WithIndexerImage sets the indexer container image.
func WithIndexerImage(image string) Option {
	return func(c *config) {
		c.indexerImage = image
	}
}

// WithDelegatorImage sets the delegator container image.
func WithDelegatorImage(image string) Option {
	return func(c *config) {
		c.delegatorImage = image
	}
}

// WithUploadImage sets the upload service container image.
func WithUploadImage(image string) Option {
	return func(c *config) {
		c.uploadImage = image
	}
}

// WithSignerImage sets the signing service container image.
func WithSignerImage(image string) Option {
	return func(c *config) {
		c.signerImage = image
	}
}

// WithBlockchainImage sets the blockchain (Anvil) container image.
func WithBlockchainImage(image string) Option {
	return func(c *config) {
		c.blockchainImage = image
	}
}

// WithIPNIImage sets the IPNI container image.
func WithIPNIImage(image string) Option {
	return func(c *config) {
		c.ipniImage = image
	}
}

// WithTimeout sets the maximum time to wait for the stack to start.
func WithTimeout(d time.Duration) Option {
	return func(c *config) {
		c.timeout = d
	}
}

// WithKeepOnFailure prevents cleanup when a test fails, useful for debugging.
func WithKeepOnFailure() Option {
	return func(c *config) {
		c.keepOnFailure = true
	}
}

// WithPiriCount configures N identical piri nodes with default storage settings.
//
// Example:
//
//	s := stack.MustNewStack(t, stack.WithPiriCount(3))
func WithPiriCount(n int) Option {
	return func(c *config) {
		c.piriNodes = make([]PiriNodeConfig, n)
	}
}

// WithPiriNodes configures specific piri nodes with individual settings.
//
// Example:
//
//	s := stack.MustNewStack(t, stack.WithPiriNodes(
//	    stack.PiriNodeConfig{Postgres: true, S3: true},
//	    stack.PiriNodeConfig{},
//	))
func WithPiriNodes(nodes ...PiriNodeConfig) Option {
	return func(c *config) {
		c.piriNodes = nodes
	}
}
