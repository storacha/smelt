package guppytests

import (
	"testing"

	"github.com/storacha/smelt/pkg/clients/guppy"
	"github.com/storacha/smelt/pkg/stack"
	"github.com/stretchr/testify/require"
)

func TestGuppy(t *testing.T) {
	ctx := t.Context()
	stack := stack.MustNewStack(
		t,
		stack.WithGuppyImage("ghcr.io/storacha/guppy:main-dev"),
	)
	client, err := guppy.NewContainerClient(stack)
	require.NoError(t, err)

	largeFiles, err := client.GenerateTestData(ctx, "50MB", guppy.WithMinFileSize("10MB"))
	require.NoError(t, err)
	smallFiles, err := client.GenerateTestData(ctx, "10MB", guppy.WithMinFileSize("100KB"), guppy.WithMaxFileSize("1MB"))
	require.NoError(t, err)

	err = client.Login(ctx, "hephaestus@lemnos.gr")
	require.NoError(t, err)

	spaceDID, err := client.GenerateSpace(ctx)
	require.NoError(t, err)

	// TODO: Get space info

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

	// TODO: Reset and re-retrieve
}
