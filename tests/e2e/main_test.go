//go:build e2e

package e2e

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/storacha/smelt/pkg/stack"
)

// TestMain sweeps leaked containers and volumes from prior test runs
// before any test in this package executes. Applies to every test in
// the e2e suite (smoke_test.go, snapshot_test.go, etc.) because
// Go calls TestMain once per package.
//
// Each individual test also registers `t.Cleanup` for its own stack,
// which handles the common failure modes (healthcheck timeout,
// assertion failure, etc.). This pre-sweep catches the cases `t.Cleanup`
// can't reach: SIGKILL, panic mid-cleanup, or a prior run with
// `stack.WithKeepOnFailure()` that was never torn down manually.
//
// Skipped implicitly if the docker daemon isn't reachable —
// CleanupLeaked returns an error which we just log and proceed.
func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	if err := stack.CleanupLeaked(ctx); err != nil {
		log.Printf("smeltery: pre-test sweep warning: %v", err)
	}
	cancel()
	os.Exit(m.Run())
}
