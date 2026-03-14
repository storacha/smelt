package stack

import "time"

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

	// Piri storage profiles
	piriPostgres bool // Use PostgreSQL instead of SQLite
	piriS3       bool // Use S3 (MinIO) instead of filesystem

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

	// Piri storage backend configuration
	if c.piriPostgres {
		env["PIRI_DB_BACKEND"] = "postgres"
	}
	if c.piriS3 {
		env["PIRI_BLOB_BACKEND"] = "s3"
	}

	return env
}

// buildProfiles returns the list of Docker Compose profiles to enable.
func (c *config) buildProfiles() []string {
	var profiles []string
	if c.piriPostgres {
		profiles = append(profiles, "piri-postgres")
	}
	if c.piriS3 {
		profiles = append(profiles, "piri-s3")
	}
	return profiles
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

// WithPiriPostgres enables the PostgreSQL database backend for piri.
// This starts an additional piri-postgres service and configures piri to use
// PostgreSQL instead of the default SQLite database.
//
// Example:
//
//	s := stack.MustNewStack(t, stack.WithPiriPostgres())
func WithPiriPostgres() Option {
	return func(c *config) {
		c.piriPostgres = true
	}
}

// WithPiriS3 enables the S3 (MinIO) blob storage backend for piri.
// This starts an additional piri-minio service and configures piri to use
// S3-compatible storage instead of the default filesystem storage.
//
// Example:
//
//	s := stack.MustNewStack(t, stack.WithPiriS3())
func WithPiriS3() Option {
	return func(c *config) {
		c.piriS3 = true
	}
}
