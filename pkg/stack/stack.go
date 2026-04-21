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
	osexec "os/exec"
	"os/user"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
	_ "github.com/lib/pq" // postgres driver for wait.ForSQL
	"github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/modules/compose"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/storacha/smelt/pkg/generate"
	"github.com/storacha/smelt/pkg/manifest"
	"github.com/storacha/smelt/pkg/snapshot"
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

	// 1. Extract embedded files to temp directory
	tempDir, err := extractFiles(t)
	if err != nil {
		return nil, fmt.Errorf("extract files: %w", err)
	}

	// If the caller picked an embedded snapshot, materialize it on disk
	// so the rest of the flow can treat it like any path-based snapshot.
	// Prefer this path over WithSnapshot for external consumers — they
	// don't have smelt's snapshots/ dir in their checkout, but the
	// embedded FS travels with the Go import.
	if cfg.embeddedSnapshotName != "" {
		if cfg.snapshotPath != "" {
			return nil, fmt.Errorf("WithEmbeddedSnapshot and WithSnapshot are mutually exclusive")
		}
		extracted, err := extractEmbeddedSnapshot(cfg.embeddedSnapshotName, tempDir)
		if err != nil {
			return nil, err
		}
		cfg.snapshotPath = extracted
	}

	// 2. Determine topology and stage filesystem state (keys/proofs/chain)
	//    either from a snapshot or by generating fresh.
	var resolvedNodes []manifest.ResolvedPiriNode
	var snapDesc *snapshot.Descriptor
	var snapDir string
	composeDir := filepath.Join(tempDir, "generated", "compose")
	if err := os.MkdirAll(composeDir, 0755); err != nil {
		return nil, fmt.Errorf("create compose dir: %w", err)
	}

	if cfg.snapshotPath != "" {
		if err := validateSnapshotOptions(cfg); err != nil {
			return nil, err
		}
		snapDir, err = resolveSnapshotDir(cfg.snapshotPath)
		if err != nil {
			return nil, err
		}
		resolvedNodes, err = loadSnapshotTopology(snapDir)
		if err != nil {
			return nil, err
		}
		snapDesc, err = snapshot.LoadFiles(ctx, snapDir, snapshot.LoadFilesPaths{
			KeysDir:    filepath.Join(tempDir, "generated", "keys"),
			ProofsDir:  filepath.Join(tempDir, "generated", "proofs"),
			ScratchDir: filepath.Join(tempDir, "generated", "snapshot-scratch"),
		})
		if err != nil {
			return nil, fmt.Errorf("load snapshot files: %w", err)
		}
		t.Logf("smeltery: booting from snapshot %s (%d piri node(s), %d volume(s))",
			snapDir, len(resolvedNodes), len(snapDesc.Volumes))
	} else {
		resolvedNodes = cfg.resolveNodes()
		keysDir := filepath.Join(tempDir, "generated", "keys")
		if err := generate.GenerateKeys(keysDir, resolvedNodes, false); err != nil {
			return nil, fmt.Errorf("generate keys: %w", err)
		}
		if err := generateProofs(tempDir, resolvedNodes); err != nil {
			return nil, fmt.Errorf("generate proofs: %w", err)
		}
		// Cold-boot: compose bind-mounts blockchain state from
		// generated/snapshot-scratch/. Seed scratch from the embedded
		// post-deploy baseline so the bind-mount source exists as a file
		// (not an auto-created root-owned dir) before compose.Up.
		if err := seedBaselineState(tempDir); err != nil {
			return nil, fmt.Errorf("seed baseline state: %w", err)
		}
	}

	// Generate piri compose YAML — driven by whichever topology we resolved
	// above (from the snapshot's smelt.yml or from WithPiri* options).
	piriYAML, err := generate.GeneratePiriCompose(resolvedNodes)
	if err != nil {
		return nil, fmt.Errorf("generate piri compose: %w", err)
	}
	if err := os.WriteFile(filepath.Join(composeDir, "piri.yml"), piriYAML, 0644); err != nil {
		return nil, fmt.Errorf("write piri compose: %w", err)
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

	// 7. Create compose stack with optional profiles. The project name is
	// deterministic per-test so we know volume names in advance — required
	// for snapshot-based restore to populate volumes BEFORE compose.Up.
	projectName := "smeltery-" + sanitizeTestName(t.Name())
	composeOpts := []compose.ComposeStackOption{
		compose.StackIdentifier(projectName),
		compose.WithStackFiles(composeFiles...),
	}

	composeStack, err := compose.NewDockerComposeWith(composeOpts...)
	if err != nil {
		return nil, fmt.Errorf("create compose: %w", err)
	}

	stack := &Stack{
		t:         t,
		compose:   composeStack,
		tempDir:   tempDir,
		cfg:       cfg,
		piriNodes: resolvedNodes,
	}

	// Register cleanup BEFORE compose.Up. If Up fails (e.g., a container
	// healthcheck times out), the half-started stack still needs tearing
	// down — otherwise it leaks containers into the developer's Docker.
	// t.Cleanup runs whether the test passes, fails, or returns early.
	t.Cleanup(func() {
		if cfg.keepOnFailure && t.Failed() {
			t.Logf("smeltery: keeping stack running due to test failure (tempDir: %s)", tempDir)
			return
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := stack.Close(closeCtx); err != nil {
			t.Logf("smeltery: stack cleanup failed: %v", err)
		}
	})

	// 7b. If restoring from snapshot, pre-populate docker volumes so they
	// exist with the expected labels and content before compose.Up.
	if snapDesc != nil {
		volsSrc := filepath.Join(snapDir, "volumes")
		for _, v := range snapDesc.Volumes {
			if err := snapshot.RestoreVolume(ctx, projectName, v, volsSrc); err != nil {
				return nil, fmt.Errorf("restore volume %s: %w", v, err)
			}
		}
	}

	// 8. Start with wait strategies
	startCtx := ctx
	if cfg.timeout > 0 {
		var cancel context.CancelFunc
		startCtx, cancel = context.WithTimeout(ctx, cfg.timeout)
		defer cancel()
	}

	// Build wait strategies for core services
	waitStack := composeStack.
		WithEnv(env).
		WaitForService("blockchain", wait.ForListeningPort("8545/tcp").WithStartupTimeout(2*time.Minute)).
		WaitForService("upload", wait.ForHTTP("/health").WithPort("80/tcp").WithStartupTimeout(2*time.Minute)).
		WaitForService("indexer", wait.ForHTTP("/").WithPort("80/tcp").WithStartupTimeout(2*time.Minute)).
		WaitForService("delegator", wait.ForHTTP("/healthcheck").WithPort("80/tcp").WithStartupTimeout(2*time.Minute)).
		WaitForService("email", wait.ForHTTP("/api/server").WithPort("80/tcp").WithStartupTimeout(2*time.Minute))

	// Wait for all piri nodes
	for _, node := range resolvedNodes {
		waitStack = waitStack.WaitForService(node.Name,
			wait.ForHTTP("/readyz").WithPort("3000/tcp").WithStartupTimeout(3*time.Minute))
	}

	// Up failures propagate up; the t.Cleanup registered above handles
	// teardown so no matter where Up fails (container healthcheck, wait
	// timeout, docker daemon hiccup), the half-started stack gets cleaned
	// up at test end.
	if err := waitStack.Up(startCtx, compose.Wait(true)); err != nil {
		return nil, fmt.Errorf("start stack: %w", err)
	}

	return stack, nil
}

// MustNewStack creates and starts a network, calling t.Fatal on error.
func MustNewStack(t *testing.T, opts ...Option) *Stack {
	t.Helper()
	stack, err := NewStack(context.Background(), t, opts...)
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
	if s.compose == nil {
		return nil
	}
	err := s.compose.Down(ctx,
		compose.RemoveOrphans(true),
		compose.RemoveVolumes(true),
	)
	// Blockchain's SIGTERM trap writes the /output scratch files as root.
	// Without this chown, t.TempDir cleanup hits unlinkat EPERM and leaks
	// the tempDir on disk. Best-effort — a cleanup failure here isn't
	// worth failing the test over.
	s.chownScratchToHostUser(ctx)
	return err
}

// chownScratchToHostUser runs a busybox container as root to chown the
// scratch dir contents back to the host user, so `t.TempDir()`'s
// post-test cleanup can unlink them. Docker bind mounts preserve host
// UID/GID, so chown inside the container changes the host file's owner.
func (s *Stack) chownScratchToHostUser(ctx context.Context) {
	scratchDir := filepath.Join(s.tempDir, "generated", "snapshot-scratch")
	if _, err := os.Stat(scratchDir); err != nil {
		return
	}
	u, err := user.Current()
	if err != nil {
		return
	}
	cmd := osexec.CommandContext(ctx, "docker", "run", "--rm", "-u", "0:0",
		"-v", fmt.Sprintf("%s:/s", scratchDir),
		"busybox", "chown", "-R", u.Uid+":"+u.Gid, "/s",
	)
	_ = cmd.Run()
}

// PiriEndpointN returns the HTTP endpoint for the Nth piri node.
func (s *Stack) PiriEndpointN(index int) string {
	name := fmt.Sprintf("piri-%d", index)
	container, err := s.compose.ServiceContainer(context.Background(), name)
	if err != nil {
		s.t.Fatalf("get %s container: %v", name, err)
	}
	host, err := container.Host(context.Background())
	if err != nil {
		s.t.Fatalf("get %s host: %v", name, err)
	}
	port, err := container.MappedPort(context.Background(), "3000/tcp")
	if err != nil {
		s.t.Fatalf("get %s port: %v", name, err)
	}
	return fmt.Sprintf("http://%s:%s", host, port.Port())
}

// PiriCount returns the number of piri nodes in the stack.
func (s *Stack) PiriCount() int {
	return len(s.piriNodes)
}

// EmailEndpoint returns the HTTP API endpoint for the email service.
func (s *Stack) EmailEndpoint() string {
	container, err := s.compose.ServiceContainer(context.Background(), "email")
	if err != nil {
		s.t.Fatalf("getting email container: %v", err)
	}
	host, err := container.Host(context.Background())
	if err != nil {
		s.t.Fatalf("getting email host: %v", err)
	}
	port, err := container.MappedPort(context.Background(), "80/tcp")
	if err != nil {
		s.t.Fatalf("getting email port: %v", err)
	}
	return fmt.Sprintf("http://%s:%s", host, port.Port())
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
