package guppytests

import (
	"os"
	"testing"

	"github.com/storacha/smelt/pkg/clients/guppy"
	"github.com/storacha/smelt/pkg/stack"
	"github.com/stretchr/testify/require"
)

func TestGuppy(t *testing.T) {
	ctx := t.Context()
	opts := []stack.Option{
		stack.WithGuppyImage("ghcr.io/storacha/guppy:main-dev"),
	}
	if os.Getenv("CI") == "" {
		opts = append(opts, stack.WithEmbeddedSnapshot("3-piri-filesystem-sqlite"))
	}
	stack := stack.MustNewStack(t, opts...)
	client, err := guppy.NewContainerClient(stack)
	require.NoError(t, err)

	err = client.Login(ctx, "hephaestus@lemnos.gr")
	require.NoError(t, err)

	t.Run("can get space info", func(t *testing.T) {
		spaceDID, err := client.GenerateSpace(ctx)
		require.NoError(t, err)

		spaceInfo, err := client.GetSpaceInfo(ctx, spaceDID)
		require.NoError(t, err)
		t.Logf("space info: %+v", spaceInfo)
		require.Equal(t, spaceDID, spaceInfo.DID)
	})

	t.Run("can upload and retrieve content", func(t *testing.T) {
		spaceDID, err := client.GenerateSpace(ctx)
		require.NoError(t, err)

		largeFiles, err := client.GenerateTestData(ctx, "50MB", guppy.WithMinFileSize("10MB"))
		require.NoError(t, err)
		smallFiles, err := client.GenerateTestData(ctx, "10MB", guppy.WithMinFileSize("100KB"), guppy.WithMaxFileSize("1MB"))
		require.NoError(t, err)

		err = client.AddSource(ctx, spaceDID, largeFiles)
		require.NoError(t, err)
		err = client.AddSource(ctx, spaceDID, smallFiles)
		require.NoError(t, err)

		uploadInfos, err := client.Upload(ctx, spaceDID)
		require.NoError(t, err)

		// TODO: Check local upload state

		// TODO: Verify upload

		for _, info := range uploadInfos {
			err = client.Retrieve(ctx, spaceDID, info.CID, "retrieved/"+info.CID)
			require.NoError(t, err)

			// TODO: Verify retrieved content
		}
	})

	// TODO: Reset and re-retrieve
}
