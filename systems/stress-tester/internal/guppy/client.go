package guppy

import (
	"context"
)

// Client defines the interface for interacting with guppy
type Client interface {
	// Login logs in with the given email
	Login(ctx context.Context, email string) error

	// GenerateSpace creates a new space and returns its DID
	GenerateSpace(ctx context.Context, provisionTo string) (spaceDID string, err error)

	// AddSource adds a source directory to a space
	AddSource(ctx context.Context, spaceDID, path string) error

	// Upload uploads all sources in a space and returns the CIDs
	Upload(ctx context.Context, spaceDID string) (cids []string, err error)

	// Retrieve downloads content by CID to a destination path
	Retrieve(ctx context.Context, spaceDID, cid, destPath string) error

	// Verify verifies the integrity of a DAG by its root CID
	Verify(ctx context.Context, cid string) error
}
