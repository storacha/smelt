// Package stack provides a simple API for spinning up the complete Storacha
// network in Go tests using testcontainers-go.
//
// Example usage:
//
//	func TestUploadFlow(t *testing.T) {
//	    s := stack.MustNewStack(t,
//	        stack.WithPiriImage("my-piri:test"),
//	    )
//
//	    resp, _ := http.Get(s.PiriEndpointN(0) + "/readyz")
//	    assert.Equal(t, 200, resp.StatusCode)
//	}
package stack

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/pkg/stdcopy"
	_ "github.com/lib/pq" // postgres driver for wait.ForSQL
	"github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/modules/compose"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/storacha/smelt/pkg/generate"
	"github.com/storacha/smelt/pkg/manifest"
)

// Stack represents a running Storacha network.
type Stack struct {
	t         *testing.T
	compose   compose.ComposeStack
	tempDir   string
	cfg       *config
	piriNodes []manifest.ResolvedPiriNode
}

// NewStack creates and starts a complete Storacha network.
// Returns error if startup fails. Cleanup is automatically registered via t.Cleanup().
func NewStack(ctx context.Context, t *testing.T, opts ...Option) (*Stack, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	// 1. Extract embedded files
	var tempDir string
	if cfg.keep {
		// Disable Ryuk (testcontainers' reaper) so containers survive after the test process exits.
		t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

		tempDir = filepath.Join(os.TempDir(), "smelt-stack")
		if err := os.MkdirAll(tempDir, 0755); err != nil {
			return nil, fmt.Errorf("create stable dir: %w", err)
		}
		if _, err := extractFilesToDir(tempDir); err != nil {
			return nil, fmt.Errorf("extract files: %w", err)
		}
	} else {
		var err error
		tempDir, err = extractFiles(t)
		if err != nil {
			return nil, fmt.Errorf("extract files: %w", err)
		}
	}

	// 2. Resolve piri node configuration and generate compose + keys.
	resolvedNodes := cfg.resolveNodes()

	keysDir := filepath.Join(tempDir, "generated", "keys")
	if err := generate.GenerateKeys(keysDir, resolvedNodes, false); err != nil {
		return nil, fmt.Errorf("generate keys: %w", err)
	}

	// Generate piri compose YAML.
	piriYAML, err := generate.GeneratePiriCompose(resolvedNodes)
	if err != nil {
		return nil, fmt.Errorf("generate piri compose: %w", err)
	}
	composeDir := filepath.Join(tempDir, "generated", "compose")
	if err := os.MkdirAll(composeDir, 0755); err != nil {
		return nil, fmt.Errorf("create compose dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(composeDir, "piri.yml"), piriYAML, 0644); err != nil {
		return nil, fmt.Errorf("write piri compose: %w", err)
	}

	// 3. Generate UCAN delegation proofs (per-node piri → upload + static indexer/etracker)
	if err := generateProofs(tempDir, resolvedNodes); err != nil {
		return nil, fmt.Errorf("generate proofs: %w", err)
	}

	// 4. Ensure Docker network exists
	if err := ensureNetwork(ctx, "storacha-network"); err != nil {
		return nil, fmt.Errorf("ensure network: %w", err)
	}

	// 5. Build environment with image overrides
	env := cfg.buildEnv()

	// 6. Prepare compose files (main + any overrides)
	composePath := filepath.Join(tempDir, "compose.yml")
	composeFiles := []string{composePath}

	// Generate override file for binary mounts if needed
	if cfg.piriBinaryPath != "" {
		// Verify binary exists
		if _, err := os.Stat(cfg.piriBinaryPath); err != nil {
			return nil, fmt.Errorf("piri binary not found at %s: %w", cfg.piriBinaryPath, err)
		}

		overridePath, err := generateBinaryOverride(tempDir, cfg, resolvedNodes)
		if err != nil {
			return nil, fmt.Errorf("generate binary override: %w", err)
		}
		composeFiles = append(composeFiles, overridePath)
		t.Logf("smeltery: mounting local piri binary from %s", cfg.piriBinaryPath)
	}

	// 7. Create compose stack with optional profiles
	stackID := "smeltery-" + sanitizeTestName(t.Name())
	if cfg.keep {
		stackID = "smeltery"
	}
	composeOpts := []compose.ComposeStackOption{
		compose.StackIdentifier(stackID),
		compose.WithStackFiles(composeFiles...),
	}

	composeStack, err := compose.NewDockerComposeWith(composeOpts...)
	if err != nil {
		return nil, fmt.Errorf("create compose: %w", err)
	}

	// 8. Start with wait strategies
	startCtx := ctx
	if cfg.timeout > 0 {
		var cancel context.CancelFunc
		startCtx, cancel = context.WithTimeout(ctx, cfg.timeout)
		defer cancel()
	}

	// Build wait strategies using Docker healthchecks (no host port mappings needed)
	waitStack := composeStack.
		WithEnv(env).
		WaitForService("blockchain", wait.ForHealthCheck().WithStartupTimeout(2*time.Minute)).
		WaitForService("upload", wait.ForHealthCheck().WithStartupTimeout(2*time.Minute)).
		WaitForService("indexer", wait.ForHealthCheck().WithStartupTimeout(2*time.Minute)).
		WaitForService("delegator", wait.ForHealthCheck().WithStartupTimeout(2*time.Minute)).
		WaitForService("email", wait.ForHealthCheck().WithStartupTimeout(2*time.Minute))

	// Wait for all piri nodes
	for _, node := range resolvedNodes {
		waitStack = waitStack.WaitForService(node.Name,
			wait.ForHealthCheck().WithStartupTimeout(3*time.Minute))
	}

	upOpts := []compose.StackUpOption{compose.Wait(true)}
	if cfg.keep {
		upOpts = append(upOpts, compose.WithRecreate("diverged"))
	}
	err = waitStack.Up(startCtx, upOpts...)
	if err != nil {
		// Clean up containers on startup failure (but not in keep mode)
		if !cfg.keep {
			_ = composeStack.Down(ctx,
				compose.RemoveOrphans(true),
				compose.RemoveVolumes(true),
			)
		}
		return nil, fmt.Errorf("start stack: %w", err)
	}

	stack := &Stack{
		t:         t,
		compose:   composeStack,
		tempDir:   tempDir,
		cfg:       cfg,
		piriNodes: resolvedNodes,
	}

	// 9. Register cleanup
	if cfg.keep {
		t.Logf("smeltery: SMELT_KEEP is set, stack will persist after test (dir: %s, project: %s)", tempDir, stackID)
	} else {
		t.Cleanup(func() {
			if cfg.keepOnFailure && t.Failed() {
				t.Logf("smeltery: keeping stack running due to test failure (tempDir: %s)", tempDir)
				return
			}
			stack.Close(context.Background())
		})
	}

	return stack, nil
}

// MustNewStack creates and starts a network, calling t.Fatal on error.
func MustNewStack(t *testing.T, opts ...Option) *Stack {
	t.Helper()
	stack, err := NewStack(t.Context(), t, opts...)
	if err != nil {
		t.Fatalf("smeltery: failed to create stack: %v", err)
	}
	return stack
}

// Logs returns the logs for a service.
func (s *Stack) Logs(ctx context.Context, service string) (string, error) {
	container, err := s.compose.ServiceContainer(ctx, service)
	if err != nil {
		return "", fmt.Errorf("get container for %s: %w", service, err)
	}

	logs, err := container.Logs(ctx)
	if err != nil {
		return "", fmt.Errorf("get logs for %s: %w", service, err)
	}
	defer logs.Close()

	// Read all logs
	buf := make([]byte, 0, 1024*1024) // 1MB buffer
	tmp := make([]byte, 4096)
	for {
		n, err := logs.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}

	return string(buf), nil
}

// Exec executes a command inside a service container and returns stdout and stderr separately.
func (s *Stack) Exec(ctx context.Context, service string, args ...string) (stdout, stderr string, err error) {
	container, err := s.compose.ServiceContainer(ctx, service)
	if err != nil {
		return "", "", fmt.Errorf("get container for %s: %w", service, err)
	}

	exitCode, reader, err := container.Exec(ctx, args, exec.WithUser("root"))
	if err != nil {
		return "", "", fmt.Errorf("exec command: %w", err)
	}

	// Demultiplex stdout and stderr from Docker's multiplexed stream
	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, reader); err != nil {
		return "", "", fmt.Errorf("read output: %w", err)
	}

	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()

	if exitCode != 0 {
		return stdout, stderr, fmt.Errorf("command failed with exit code %d: stdout=%s stderr=%s", exitCode, stdout, stderr)
	}

	return stdout, stderr, nil
}

// Close shuts down the stack and cleans up resources.
// This is called automatically via t.Cleanup(), but can be called manually.
func (s *Stack) Close(ctx context.Context) error {
	if s.compose != nil {
		return s.compose.Down(ctx,
			compose.RemoveOrphans(true),
			compose.RemoveVolumes(true),
		)
	}
	return nil
}

// PiriEndpointN returns the HTTP endpoint for the Nth piri node.
func (s *Stack) PiriEndpointN(index int) string {
	ctx := s.t.Context()
	name := fmt.Sprintf("piri-%d", index)
	container, err := s.compose.ServiceContainer(ctx, name)
	if err != nil {
		s.t.Fatalf("get %s container: %v", name, err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		s.t.Fatalf("get %s host: %v", name, err)
	}
	port, err := container.MappedPort(ctx, "3000/tcp")
	if err != nil {
		if errdefs.IsNotFound(err) {
			s.t.Fatalf("service %q does not host-map its 3000/tcp port.", name)
		}
		s.t.Fatalf("get %s port: %v", name, err)
	}
	s.t.Logf("http://%s:%s", host, port.Port())
	return fmt.Sprintf("http://%s:%s", host, port.Port())
}

// PiriCount returns the number of piri nodes in the stack.
func (s *Stack) PiriCount() int {
	return len(s.piriNodes)
}

// generateBinaryOverride creates a compose override file that mounts local binaries
// into containers, replacing the binaries from the images.
func generateBinaryOverride(tempDir string, cfg *config, nodes []manifest.ResolvedPiriNode) (string, error) {
	overridePath := filepath.Join(tempDir, "compose.override.yml")

	var content string
	content = "# Auto-generated binary mount overrides\nservices:\n"

	if cfg.piriBinaryPath != "" {
		absPath, err := filepath.Abs(cfg.piriBinaryPath)
		if err != nil {
			return "", fmt.Errorf("get absolute path: %w", err)
		}
		for _, node := range nodes {
			content += fmt.Sprintf(`  %s:
    volumes:
      - %s:/usr/bin/piri:ro
`, node.Name, absPath)
		}
	}

	if err := os.WriteFile(overridePath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write override file: %w", err)
	}

	return overridePath, nil
}

// sanitizeTestName converts a test name to a valid Docker Compose project name
// (lowercase alphanumeric, hyphens, underscores, must start with letter/number)
func sanitizeTestName(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		// Convert uppercase to lowercase
		if c >= 'A' && c <= 'Z' {
			c = c + 32 // ASCII lowercase conversion
		}
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			result = append(result, c)
		} else {
			result = append(result, '-')
		}
	}
	return string(result)
}
