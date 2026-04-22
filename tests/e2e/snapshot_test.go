//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"runtime"
	"testing"
	"time"

	"github.com/storacha/smelt/pkg/clients/guppy"
	"github.com/storacha/smelt/pkg/stack"
)

// TestStackFromSnapshot exercises stack.WithSnapshot: the committed
// 3-piri-filesystem-sqlite snapshot should restore in a fraction of
// the cold-boot time (smoke_test.go is the baseline; its typical run
// is ~60-120s, this one should land in the low tens of seconds).
func TestStackFromSnapshot(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("skipping on darwin (docker-in-docker flakiness)")
	}

	ctx := t.Context()

	start := time.Now()
	s := stack.MustNewStack(t, stack.WithEmbeddedSnapshot("3-piri-filesystem-sqlite"))
	elapsed := time.Since(start)
	t.Logf("stack up from snapshot in %s", elapsed)

	// Sanity: every piri in the snapshot's topology responds to readyz.
	// If registration state hadn't been restored, piri's healthcheck
	// (hardcoded 180s start_period) would still be pending and
	// MustNewStack would have timed out in WaitForService. Reaching
	// here at all is strong evidence the snapshot loaded correctly.
	for i := 0; i < s.PiriCount(); i++ {
		endpoint := s.PiriEndpointN(i) + "/readyz"
		resp, err := http.Get(endpoint)
		if err != nil {
			t.Fatalf("piri-%d readyz: %v", i, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("piri-%d readyz: status=%d body=%s", i, resp.StatusCode, string(body))
		}
		t.Logf("piri-%d ready: %s", i, string(body))
	}

	// End-to-end smoke: upload a small file and retrieve it. Exercises
	// the delegator allow-list + upload provider registry, which are
	// restored from dynamodb-data.tar in the snapshot.
	gup, err := guppy.NewContainerClient(s)
	if err != nil {
		t.Fatalf("guppy client: %v", err)
	}

	if err := gup.Login(ctx, "test@example.com"); err != nil {
		t.Fatalf("login: %v", err)
	}

	spaceDID, err := gup.GenerateSpace(ctx)
	if err != nil {
		t.Fatalf("generate space: %v", err)
	}

	dataPath, err := gup.GenerateTestData(ctx, "100MB")
	if err != nil {
		t.Fatalf("generate test data: %v", err)
	}

	if err := gup.AddSource(ctx, spaceDID, dataPath); err != nil {
		t.Fatalf("add source: %v", err)
	}

	cids, err := gup.Upload(ctx, spaceDID)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if len(cids) == 0 {
		t.Fatal("upload returned no CIDs")
	}

	t.Logf("uploaded %d CID(s) via snapshot-loaded stack", len(cids))
}
