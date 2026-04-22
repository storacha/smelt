//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/storacha/smelt/pkg/clients/guppy"
	"github.com/storacha/smelt/pkg/stack"
)

func TestUploadAndRetrieve(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("skipping on darwin (docker-in-docker flakiness)")
	}

	tests := []struct {
		name        string
		useS3       bool
		usePostgres bool
	}{
		{name: "default"},
		{name: "s3", useS3: true},
		{name: "postgres", usePostgres: true},
		{name: "s3_and_postgres", useS3: true, usePostgres: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()

			opts := []stack.Option{
				stack.WithPiriNodes(stack.PiriNodeConfig{
					S3:       tt.useS3,
					Postgres: tt.usePostgres,
				}),
			}
			if img := os.Getenv("PIRI_IMAGE"); img != "" {
				opts = append(opts, stack.WithPiriImage(img))
			}
			if img := os.Getenv("GUPPY_IMAGE"); img != "" {
				opts = append(opts, stack.WithGuppyImage(img))
			}

			s := stack.MustNewStack(t, opts...)
			gup, err := guppy.NewContainerClient(s)
			if err != nil {
				t.Fatal(err)
			}

			if err := gup.Login(ctx, "test@example.com"); err != nil {
				t.Fatalf("failed to login: %v", err)
			}

			spaceDID, err := gup.GenerateSpace(ctx)
			if err != nil {
				t.Fatalf("failed to generate space: %v", err)
			}
			t.Logf("created space: %s", spaceDID)

			dataPath, err := gup.GenerateTestData(ctx, "10MB")
			if err != nil {
				t.Fatalf("failed to generate test data: %v", err)
			}

			if err := gup.AddSource(ctx, spaceDID, dataPath); err != nil {
				t.Fatalf("failed to add source: %v", err)
			}

			cids, err := gup.Upload(ctx, spaceDID, guppy.WithReplicas(1))
			if err != nil {
				t.Fatalf("failed to upload: %v", err)
			}
			if len(cids) == 0 {
				t.Fatal("expected at least one CID from upload")
			}
			t.Logf("uploaded CIDs: %v", cids)

			dstPath := fmt.Sprintf("/tmp/testdata-download-%d", time.Now().UnixNano())
			if err := gup.Retrieve(ctx, spaceDID, cids[len(cids)-1], dstPath); err != nil {
				t.Fatalf("failed to retrieve: %v", err)
			}
		})
	}
}
