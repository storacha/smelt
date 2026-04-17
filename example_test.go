package smelt_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/storacha/smelt/pkg/clients/guppy"
	"github.com/storacha/smelt/pkg/stack"
)

func TestUploadAndRetrieve(t *testing.T) {
	ctx := t.Context()

	s := stack.MustNewStack(t, stack.WithGuppyImage("ghcr.io/storacha/guppy:main-dev"))
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

	_, err = gup.Upload(ctx, spaceDID, guppy.WithReplicas(1))
	fmt.Println("FINISHED UP BUT ERR", err)
	time.Sleep(5 * time.Minute)

	// require.NoError(t, err)
	// if len(cids) == 0 {
	// 	t.Fatal("expected at least one CID from upload")
	// }
	// t.Logf("Uploaded CIDs: %v", cids)

	// dstPath := fmt.Sprintf("/tmp/testdata-download-%d", time.Now().UnixNano())
	// err = gup.Retrieve(ctx, spaceDID, cids[len(cids)-1], dstPath)
	// require.NoError(t, err)
}
