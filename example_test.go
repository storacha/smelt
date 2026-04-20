package smelt_test

import (
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	docker_client "github.com/docker/docker/client"
	"github.com/storacha/smelt/pkg/clients/guppy"
	"github.com/storacha/smelt/pkg/stack"
)

// IsRunningInCI returns true of process is running in CI environment.
func IsRunningInCI(t testing.TB) bool {
	t.Helper()
	return os.Getenv("CI") != ""
}

// IsDockerAvailable returns true if the docker daemon is available, useful for skipping tests when docker isn't running
func IsDockerAvailable(t testing.TB) bool {
	t.Helper()
	c, err := docker_client.NewClientWithOpts(docker_client.FromEnv, docker_client.WithAPIVersionNegotiation())
	require.NoError(t, err)

	_, err = c.Info(t.Context())
	if err != nil {
		t.Logf("Docker not available for test %s: %v", t.Name(), err)
		return false
	}
	return true
}

func TestUploadAndRetrieve(t *testing.T) {
	// This test expects docker to be running in linux CI environments and fails if it's not
	if IsRunningInCI(t) && runtime.GOOS == "linux" {
		if !IsDockerAvailable(t) {
			t.Fatalf("docker is expected in CI linux testing environments, but wasn't found")
		}
	}
	// otherwise this test is running locally, skip it if docker isn't available
	if !IsDockerAvailable(t) {
		t.SkipNow()
	}

	ctx := t.Context()

	s := stack.MustNewStack(
		t,
		stack.WithGuppyImage("ghcr.io/storacha/guppy:main-dev"),
		stack.WithPiriNodes(
			stack.PiriNodeConfig{
				Postgres: false,
				S3:       false,
			},
			stack.PiriNodeConfig{
				Postgres: true,
				S3:       false,
			},
			stack.PiriNodeConfig{
				Postgres: true,
				S3:       true,
			},
		),
	)
	t.Log("Stack started successfully")

	gup := guppy.MustNewContainerClient(t, s)

	// Login
	err := gup.Login(ctx, "test@example.com", guppy.WithLoginTimeout(10*time.Second))
	require.NoError(t, err)
	t.Log("Logged in successfully")

	// Create space
	spaceDID, err := gup.GenerateSpace(ctx)
	require.NoError(t, err)
	t.Logf("Created space: %s", spaceDID)

	// Generate test data inside container (10MB)
	dataPath, err := gup.GenerateTestData(ctx, "10MB")
	require.NoError(t, err)
	t.Logf("Generated test data at: %s", dataPath)

	// Add source and upload
	err = gup.AddSource(ctx, spaceDID, dataPath)
	require.NoError(t, err)
	t.Log("Added source")

	uploads, err := gup.Upload(ctx, spaceDID, guppy.WithReplicas(1))
	require.NoError(t, err)

	cids := make([]string, len(uploads))
	for i, upload := range uploads {
		cids[i] = upload.CID
		_, err := cid.Decode(upload.CID)
		require.NoError(t, err, "invalid CID returned from upload")
	}

	if len(uploads) == 0 {
		t.Fatal("expected at least one upload")
	}
	t.Logf("Uploaded CIDs: %v", cids)

	dstPath := fmt.Sprintf("/tmp/testdata-download-%d", time.Now().UnixNano())
	err = gup.Retrieve(ctx, spaceDID, cids[len(cids)-1], dstPath)
	require.NoError(t, err)
}
