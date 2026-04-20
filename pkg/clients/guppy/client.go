// Package guppy provides a client interface for interacting with the guppy CLI.
package guppy

import (
	"context"
	"time"
)

type loginConfig struct {
	timeout time.Duration
}

type LoginOption func(*loginConfig)

func WithLoginTimeout(timeout time.Duration) LoginOption {
	return func(c *loginConfig) {
		c.timeout = timeout
	}
}

type uploadConfig struct {
	replicas int
}

type UploadOption func(*uploadConfig)

func WithReplicas(replicas int) UploadOption {
	return func(c *uploadConfig) {
		c.replicas = replicas
	}
}

// Client defines the interface for interacting with guppy.
type Client interface {
	// Login logs in with the given email.
	Login(ctx context.Context, email string, options ...LoginOption) error

	// GenerateSpace creates a new space and returns its DID.
	GenerateSpace(ctx context.Context) (spaceDID string, err error)

	// AddSource adds a source directory to a space.
	AddSource(ctx context.Context, spaceDID, path string) error

	// Upload uploads all sources in a space and returns the CIDs.
	Upload(ctx context.Context, spaceDID string, options ...UploadOption) (uploads []UploadInfo, err error)

	// Retrieve downloads content by CID to a destination path.
	Retrieve(ctx context.Context, spaceDID, cid, destPath string) error

	// GenerateTestData creates random test data and returns the path.
	// This is useful for testing - it uses randdir to create test files.
	GenerateTestData(ctx context.Context, size string, opts ...GenerateTestDataOption) (path string, err error)
}
