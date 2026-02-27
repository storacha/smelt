package handlers

import (
	"context"
	"time"

	"github.com/storacha/go-libstoracha/capabilities/upload"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/receipt/fx"
	"github.com/storacha/go-ucanto/core/result"
	"github.com/storacha/go-ucanto/core/result/failure"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/go-ucanto/principal"
	"github.com/storacha/go-ucanto/server"
	"github.com/storacha/go-ucanto/ucan"

	"github.com/storacha/smelt/mock-upload-service/pkg/state"
)

// UploadAddService defines the interface for the upload/add handler.
type UploadAddService interface {
	ID() principal.Signer
	State() state.StateStore
}

// WithUploadAddMethod registers the upload/add handler.
// This handler registers an upload (root CID + shards mapping).
func WithUploadAddMethod(s UploadAddService) server.Option {
	return server.WithServiceMethod(
		upload.AddAbility,
		server.Provide(
			upload.Add,
			func(ctx context.Context,
				cap ucan.Capability[upload.AddCaveats],
				inv invocation.Invocation,
				iCtx server.InvocationContext,
			) (result.Result[upload.AddOk, failure.IPLDBuilderFailure], fx.Effects, error) {

				spaceDID := cap.With()      // Space DID
				root := cap.Nb().Root       // Root CID of the upload
				shards := cap.Nb().Shards   // Shard CIDs (blob links)

				// Parse the space DID
				space, err := did.Parse(spaceDID)
				if err != nil {
					return result.Error[upload.AddOk, failure.IPLDBuilderFailure](
						failure.FromError(err),
					), nil, nil
				}

				// Store the upload record
				if err := s.State().PutUpload(ctx, spaceDID, &state.Upload{
					Space:   space,
					Root:    root,
					Shards:  shards,
					AddedAt: time.Now(),
				}); err != nil {
					return result.Error[upload.AddOk, failure.IPLDBuilderFailure](
						failure.FromError(err),
					), nil, nil
				}

				// Return success with the root and shards
				return result.Ok[upload.AddOk, failure.IPLDBuilderFailure](upload.AddOk{
					Root:   root,
					Shards: shards,
				}), nil, nil
			},
		),
	)
}
