// Package smelt provides embedded files for the Storacha local development environment.
package smelt

import "embed"

// EmbeddedFiles contains all compose files, configs, and scripts needed to run the stack.
// Keys and proofs are generated at runtime using go-ucanto.
//
//go:embed compose.yml .env

//go:embed systems/common/compose.yml

//go:embed systems/blockchain/compose.yml
//go:embed systems/blockchain/state/deployed-addresses.json
//go:embed systems/blockchain/state/anvil-state.json

//go:embed systems/delegator/compose.yml
//go:embed systems/delegator/config/*

//go:embed systems/guppy/compose.yml
//go:embed systems/guppy/config/*

//go:embed systems/indexing/compose.yml
//go:embed systems/indexing/ipni/compose.yml
//go:embed systems/indexing/ipni/entrypoint.sh
//go:embed systems/indexing/indexer/compose.yml

//go:embed systems/piri/entrypoint.sh
//go:embed systems/piri/register-did.sh
//go:embed systems/piri/config/*

//go:embed systems/signing-service/compose.yml
//go:embed systems/signing-service/config/*

//go:embed systems/upload/compose.yml
//go:embed systems/upload/config/*
//go:embed systems/upload/post_start.sh

// Curated snapshots shipped with the Go module so external consumers
// (importers of pkg/stack) can call stack.WithEmbeddedSnapshot without
// knowing anything about smelt's on-disk layout. New directories
// committed under snapshots/ are automatically included on next build.
//
//go:embed snapshots
var EmbeddedFiles embed.FS
