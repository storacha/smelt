package guppy

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// CLIClient wraps the guppy CLI binary
type CLIClient struct {
	binaryPath string
	configPath string
	email      string
}

// NewCLIClient creates a new guppy CLI client
func NewCLIClient(binaryPath, configPath, email string) *CLIClient {
	if binaryPath == "" {
		binaryPath = "guppy"
	}
	return &CLIClient{
		binaryPath: binaryPath,
		configPath: configPath,
		email:      email,
	}
}

func (c *CLIClient) runCommand(ctx context.Context, args ...string) (string, string, error) {
	// Add config path if specified
	if c.configPath != "" {
		args = append([]string{"--config", c.configPath}, args...)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// Login logs in with the given email
func (c *CLIClient) Login(ctx context.Context, email string) error {
	stdout, stderr, err := c.runCommand(ctx, "login", email)
	if err != nil {
		return fmt.Errorf("login failed: %w\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Check for success message
	combined := stdout + stderr
	if !strings.Contains(combined, "Successfully logged in") && !strings.Contains(combined, "already logged in") {
		return fmt.Errorf("login may have failed, output: %s", combined)
	}

	return nil
}

// EmailToDIDMailto converts an email to did:mailto format
// e.g., "user@example.com" -> "did:mailto:example.com:user"
func EmailToDIDMailto(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return email // Return as-is if not a valid email
	}
	return fmt.Sprintf("did:mailto:%s:%s", parts[1], parts[0])
}

// GenerateSpace creates a new space and returns its DID
func (c *CLIClient) GenerateSpace(ctx context.Context, provisionTo string) (string, error) {
	args := []string{"space", "generate"}
	if provisionTo != "" {
		args = append(args, "--provision-to", provisionTo)
	}

	stdout, stderr, err := c.runCommand(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("space generate failed: %w\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Space DID is returned on stdout
	spaceDID := strings.TrimSpace(stdout)
	if spaceDID == "" {
		// Try to extract from combined output
		spaceDID = extractDID(stdout + stderr)
	}

	if spaceDID == "" || !strings.HasPrefix(spaceDID, "did:") {
		return "", fmt.Errorf("failed to extract space DID from output: %s", stdout+stderr)
	}

	return spaceDID, nil
}

// AddSource adds a source directory to a space
func (c *CLIClient) AddSource(ctx context.Context, spaceDID, path string) error {
	stdout, stderr, err := c.runCommand(ctx, "upload", "source", "add", spaceDID, path)
	if err != nil {
		return fmt.Errorf("source add failed: %w\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	return nil
}

// Upload uploads all sources in a space and returns the CIDs
func (c *CLIClient) Upload(ctx context.Context, spaceDID string) ([]string, error) {
	stdout, stderr, err := c.runCommand(ctx, "upload", spaceDID)
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Extract CIDs from output (bafy... format)
	combined := stdout + stderr
	cids := extractCIDs(combined)

	return cids, nil
}

// Retrieve downloads content by CID to a destination path
func (c *CLIClient) Retrieve(ctx context.Context, spaceDID, cid, destPath string) error {
	stdout, stderr, err := c.runCommand(ctx, "retrieve", spaceDID, cid, destPath)
	if err != nil {
		return fmt.Errorf("retrieve failed: %w\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	return nil
}

// Verify verifies the integrity of a DAG by its root CID
func (c *CLIClient) Verify(ctx context.Context, cid string) error {
	stdout, stderr, err := c.runCommand(ctx, "verify", cid)
	if err != nil {
		return fmt.Errorf("verify failed: %w\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	return nil
}

// extractDID extracts a DID from text
func extractDID(text string) string {
	// Match did:key:... or did:web:...
	re := regexp.MustCompile(`did:(key|web):[a-zA-Z0-9:._-]+`)
	matches := re.FindStringSubmatch(text)
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

// extractCIDs extracts CIDs (bafy...) from text
func extractCIDs(text string) []string {
	re := regexp.MustCompile(`bafy[a-zA-Z0-9]+`)
	return re.FindAllString(text, -1)
}
