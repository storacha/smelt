package guppy

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/storacha/smelt/pkg/stack"
	"golang.org/x/sync/errgroup"
)

var errAlreadyLoggedIn = fmt.Errorf("already logged in")

type LoginValidator interface {
	// ValidateEmailLogin waits for a validation email to be sent to the given
	// address, extracts the validation link, and clicks it. Use the passed
	// context to stop waiting.
	ValidateEmailLogin(ctx context.Context, email string) error
}

type Option func(*ContainerClient)

func WithLoginValidator(validator LoginValidator) Option {
	return func(c *ContainerClient) {
		c.validator = validator
	}
}

// Compile-time check that ContainerClient implements guppy.Client.
var _ Client = (*ContainerClient)(nil)

// ContainerClient implements guppy.Client by executing commands inside the guppy container.
type ContainerClient struct {
	stack     *stack.Stack
	validator LoginValidator
}

func MustNewContainerClient(t *testing.T, stack *stack.Stack, options ...Option) *ContainerClient {
	c, err := NewContainerClient(stack, options...)
	if err != nil {
		t.Fatalf("failed to create guppy client: %v", err)
	}
	return c
}

func NewContainerClient(stack *stack.Stack, options ...Option) (*ContainerClient, error) {
	c := &ContainerClient{
		stack: stack,
	}
	for _, option := range options {
		option(c)
	}
	if c.validator == nil {
		// Fetch emails from smtp4dev over its host-mapped API port (fine with
		// ephemeral ports — MappedPort resolves it), but POST the validation
		// link from inside the Docker network. Sprue's public_url points at
		// the `upload` DNS name in test mode, so the host can't reach it.
		clicker := &ExecDoer{Stack: stack, Service: "guppy"}
		validator, err := NewSMTP4DevLoginValidator(
			stack.EmailEndpoint(),
			WithSMTP4DevLoginValidatorClicker(clicker),
		)
		if err != nil {
			return nil, err
		}
		c.validator = validator
	}
	return c, nil
}

func (c *ContainerClient) exec(ctx context.Context, args ...string) (stdout, stderr string, err error) {
	return c.stack.Exec(ctx, "guppy", args...)
}

func (c *ContainerClient) guppyExec(ctx context.Context, args ...string) (stdout, stderr string, err error) {
	args = append([]string{"guppy"}, args...)
	return c.exec(ctx, args...)
}

// Login logs in with the given email.
func (c *ContainerClient) Login(ctx context.Context, email string, options ...LoginOption) error {
	config := &loginConfig{}
	for _, option := range options {
		option(config)
	}
	if config.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, config.timeout)
		defer cancel()
	}

	g, ctx := errgroup.WithContext(ctx)
	ctx, cancel := context.WithCancelCause(ctx)

	g.Go(func() error {
		stdout, _, err := c.guppyExec(ctx, "login", email)
		if err != nil {
			return err
		}
		if strings.Contains(stdout, "already logged in") {
			// cancel trying to validate the email - no email is being sent
			cancel(errAlreadyLoggedIn)
			return nil
		}
		if !strings.Contains(stdout, "Successfully logged in") {
			return fmt.Errorf("login may have failed, output: %s", stdout)
		}
		return nil
	})

	g.Go(func() error {
		err := c.validator.ValidateEmailLogin(ctx, email)
		if err != nil {
			if !errors.Is(context.Cause(ctx), errAlreadyLoggedIn) {
				return err
			}
		}
		return nil
	})

	return g.Wait()
}

// GenerateSpace creates a new space and returns its DID.
func (c *ContainerClient) GenerateSpace(ctx context.Context) (string, error) {
	stdout, _, err := c.guppyExec(ctx, "space", "generate")
	if err != nil {
		return "", err
	}

	spaceDID := strings.TrimSpace(stdout)
	if !strings.HasPrefix(spaceDID, "did:") {
		spaceDID = extractDID(stdout)
	}
	if spaceDID == "" {
		return "", fmt.Errorf("failed to extract space DID from output: %s", stdout)
	}

	return spaceDID, nil
}

// AddSource adds a source directory to a space.
func (c *ContainerClient) AddSource(ctx context.Context, spaceDID, path string) error {
	_, _, err := c.guppyExec(ctx, "upload", "source", "add", spaceDID, path)
	return err
}

// Upload uploads all sources in a space and returns the CIDs.
func (c *ContainerClient) Upload(ctx context.Context, spaceDID string, options ...UploadOption) ([]UploadInfo, error) {
	config := &uploadConfig{}
	for _, option := range options {
		option(config)
	}

	args := []string{"upload"}
	if config.replicas > 0 {
		args = append(args, "--replicas", fmt.Sprintf("%d", config.replicas))
	}
	args = append(args, spaceDID)

	stdout, _, err := c.guppyExec(ctx, args...)
	if err != nil {
		return nil, err
	}
	return extractUploads(stdout), nil
}

// Retrieve downloads content by CID to a destination path.
func (c *ContainerClient) Retrieve(ctx context.Context, spaceDID, cid, destPath string) error {
	_, _, err := c.guppyExec(ctx, "retrieve", spaceDID, cid, destPath)
	return err
}

// generateTestDataConfig holds optional configuration for GenerateTestData.
type generateTestDataConfig struct {
	MinFileSize string
	MaxFileSize string
}

// GenerateTestDataOption configures optional parameters for GenerateTestData.
type GenerateTestDataOption func(*generateTestDataConfig)

// WithMinFileSize sets the --min-file-size flag for randdir.
func WithMinFileSize(size string) GenerateTestDataOption {
	return func(c *generateTestDataConfig) {
		c.MinFileSize = size
	}
}

// WithMaxFileSize sets the --max-file-size flag for randdir.
func WithMaxFileSize(size string) GenerateTestDataOption {
	return func(c *generateTestDataConfig) {
		c.MaxFileSize = size
	}
}

// GenerateTestData creates random test data inside the guppy container using randdir.
// Returns the path to the generated data directory within the container.
func (c *ContainerClient) GenerateTestData(ctx context.Context, size string, opts ...GenerateTestDataOption) (string, error) {
	var cfg generateTestDataConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	// Generate unique directory name
	path := fmt.Sprintf("/tmp/testdata-%d", time.Now().UnixNano())

	args := []string{"randdir", "--size", size, "--output", path}
	if cfg.MinFileSize != "" {
		args = append(args, "--min-file-size", cfg.MinFileSize)
	}
	if cfg.MaxFileSize != "" {
		args = append(args, "--max-file-size", cfg.MaxFileSize)
	}

	// Use randdir to generate test data inside the container
	_, _, err := c.exec(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("generate test data: %w", err)
	}

	return path, nil
}

// extractDID extracts a DID from text.
func extractDID(text string) string {
	re := regexp.MustCompile(`did:(key|web):[a-zA-Z0-9:._-]+`)
	return re.FindString(text)
}

type UploadInfo struct {
	CID        string
	SourceName string
}

// extractUploads extracts CIDs (bafy...) from text.
func extractUploads(text string) []UploadInfo {
	re := regexp.MustCompile(`(bafy[a-zA-Z0-9]+) \(source: (.+)\)`)
	matches := re.FindAllStringSubmatch(text, -1)
	var uploads []UploadInfo
	for _, match := range matches {
		if len(match) == 3 {
			uploads = append(uploads, UploadInfo{
				CID:        match[1],
				SourceName: match[2],
			})
		}
	}
	return uploads
}
