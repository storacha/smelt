package stack

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

// BuildImage builds a Docker image from the repo's Dockerfile.
// Returns the image tag. The image is automatically cleaned up when the test completes.
//
// This enables testing local code changes against the full smelt stack:
//
//	func TestWithLocalChanges(t *testing.T) {
//	    localPiri := stack.BuildImage(t, "..", "local-piri")
//	    s := stack.MustNewStack(t, stack.WithPiriImage(localPiri))
//	    // ... test against local changes
//	}
func BuildImage(t *testing.T, repoPath string, imageName string) string {
	t.Helper()

	// Create unique tag for this test run
	tag := fmt.Sprintf("%s:smelt-test-%d", imageName, time.Now().UnixNano())

	t.Logf("Building Docker image %s from %s...", tag, repoPath)

	cmd := exec.Command("docker", "build", "-t", tag, ".")
	cmd.Dir = repoPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to build Docker image: %v", err)
	}

	// Cleanup image after test
	t.Cleanup(func() {
		t.Logf("Cleaning up Docker image %s", tag)
		_ = exec.Command("docker", "rmi", tag).Run()
	})

	t.Logf("Successfully built image: %s", tag)
	return tag
}

// BuildPiriImage builds piri from a local repo and returns the image tag.
// The image is automatically cleaned up when the test completes.
//
// Example:
//
//	func TestWithLocalPiri(t *testing.T) {
//	    localPiri := stack.BuildPiriImage(t, "..") // parent dir is repo root
//	    s := stack.MustNewStack(t, stack.WithPiriImage(localPiri))
//	}
func BuildPiriImage(t *testing.T, repoPath string) string {
	t.Helper()
	return BuildImage(t, repoPath, "local-piri")
}

// BuildGuppyImage builds guppy from a local repo and returns the image tag.
// The image is automatically cleaned up when the test completes.
//
// Example:
//
//	func TestWithLocalGuppy(t *testing.T) {
//	    localGuppy := stack.BuildGuppyImage(t, "..")
//	    s := stack.MustNewStack(t, stack.WithGuppyImage(localGuppy))
//	}
func BuildGuppyImage(t *testing.T, repoPath string) string {
	t.Helper()
	return BuildImage(t, repoPath, "local-guppy")
}

// BuildIndexerImage builds the indexing-service from a local repo and returns the image tag.
// The image is automatically cleaned up when the test completes.
func BuildIndexerImage(t *testing.T, repoPath string) string {
	t.Helper()
	return BuildImage(t, repoPath, "local-indexer")
}

// BuildDelegatorImage builds the delegator from a local repo and returns the image tag.
// The image is automatically cleaned up when the test completes.
func BuildDelegatorImage(t *testing.T, repoPath string) string {
	t.Helper()
	return BuildImage(t, repoPath, "local-delegator")
}

// BuildUploadImage builds the upload service from a local repo and returns the image tag.
// The image is automatically cleaned up when the test completes.
func BuildUploadImage(t *testing.T, repoPath string) string {
	t.Helper()
	return BuildImage(t, repoPath, "local-upload")
}
