package guppy

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/storacha/smelt/pkg/stack"
)

// Compile-time check that ContainerClient implements guppy.Client.
var _ Client = (*ContainerClient)(nil)

// ContainerClient implements guppy.Client by executing commands inside the guppy container.
type ContainerClient struct {
	stack *stack.Stack
}

func NewContainerClient(stack *stack.Stack) *ContainerClient {
	return &ContainerClient{
		stack: stack,
	}
}

func (c *ContainerClient) exec(ctx context.Context, args ...string) (stdout, stderr string, err error) {
	return c.stack.Exec(ctx, "guppy", args...)
}

func (c *ContainerClient) guppyExec(ctx context.Context, args ...string) (stdout, stderr string, err error) {
	args = append([]string{"guppy"}, args...)
	return c.exec(ctx, args...)
}

// Login logs in with the given email.
func (c *ContainerClient) Login(ctx context.Context, email string) error {
	stdout, _, err := c.guppyExec(ctx, "login", email)
	if err != nil {
		return err
	}

	// Check for success indicators
	if !strings.Contains(stdout, "Successfully logged in") && !strings.Contains(stdout, "already logged in") {
		return fmt.Errorf("login may have failed, output: %s", stdout)
	}

	return nil
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
func (c *ContainerClient) Upload(ctx context.Context, spaceDID string) ([]string, error) {
	stdout, _, err := c.guppyExec(ctx, "upload", spaceDID)
	if err != nil {
		return nil, err
	}
	return extractCIDs(stdout), nil
}

// Retrieve downloads content by CID to a destination path.
func (c *ContainerClient) Retrieve(ctx context.Context, spaceDID, cid, destPath string) error {
	_, _, err := c.guppyExec(ctx, "retrieve", spaceDID, cid, destPath)
	return err
}

// GenerateTestData creates random test data inside the guppy container using randdir.
// Returns the path to the generated data directory within the container.
func (c *ContainerClient) GenerateTestData(ctx context.Context, size string) (string, error) {
	// Generate unique directory name
	path := fmt.Sprintf("/tmp/testdata-%d", time.Now().UnixNano())

	// Use randdir to generate test data inside the container
	_, _, err := c.exec(ctx, "randdir", "--size", size, "--output", path)
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

// extractCIDs extracts CIDs (bafy...) from text.
func extractCIDs(text string) []string {
	re := regexp.MustCompile(`bafy[a-zA-Z0-9]+`)
	return re.FindAllString(text, -1)
}
