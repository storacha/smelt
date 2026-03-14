package stack

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildImageTagFormat(t *testing.T) {
	// Skip if Docker is not available
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found in PATH")
	}

	// Create a minimal Dockerfile in a temp directory
	tempDir := t.TempDir()
	dockerfile := `FROM alpine:latest
CMD ["echo", "test"]
`
	if err := writeFile(tempDir, "Dockerfile", dockerfile); err != nil {
		t.Fatalf("failed to create Dockerfile: %v", err)
	}

	tag := BuildImage(t, tempDir, "test-image")

	// Verify tag format
	if !strings.HasPrefix(tag, "test-image:smelt-test-") {
		t.Errorf("expected tag to start with 'test-image:smelt-test-', got %s", tag)
	}

	// Verify image exists
	cmd := exec.Command("docker", "image", "inspect", tag)
	if err := cmd.Run(); err != nil {
		t.Errorf("expected image %s to exist", tag)
	}
}

func TestBuildPiriImageTagFormat(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found in PATH")
	}

	tempDir := t.TempDir()
	dockerfile := `FROM alpine:latest
CMD ["echo", "piri"]
`
	if err := writeFile(tempDir, "Dockerfile", dockerfile); err != nil {
		t.Fatalf("failed to create Dockerfile: %v", err)
	}

	tag := BuildPiriImage(t, tempDir)

	if !strings.HasPrefix(tag, "local-piri:smelt-test-") {
		t.Errorf("expected tag to start with 'local-piri:smelt-test-', got %s", tag)
	}
}

func TestBuildGuppyImageTagFormat(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found in PATH")
	}

	tempDir := t.TempDir()
	dockerfile := `FROM alpine:latest
CMD ["echo", "guppy"]
`
	if err := writeFile(tempDir, "Dockerfile", dockerfile); err != nil {
		t.Fatalf("failed to create Dockerfile: %v", err)
	}

	tag := BuildGuppyImage(t, tempDir)

	if !strings.HasPrefix(tag, "local-guppy:smelt-test-") {
		t.Errorf("expected tag to start with 'local-guppy:smelt-test-', got %s", tag)
	}
}

func writeFile(dir, name, content string) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
}
