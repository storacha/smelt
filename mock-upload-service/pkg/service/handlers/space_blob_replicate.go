package handlers

import (
	"context"
	"log"

	spaceblobcap "github.com/storacha/go-libstoracha/capabilities/space/blob"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/receipt/fx"
	"github.com/storacha/go-ucanto/core/result"
	"github.com/storacha/go-ucanto/core/result/failure"
	"github.com/storacha/go-ucanto/principal"
	"github.com/storacha/go-ucanto/server"
	"github.com/storacha/go-ucanto/ucan"
)

// SpaceBlobReplicateService defines the interface for the space/blob/replicate handler.
type SpaceBlobReplicateService interface {
	ID() principal.Signer
}

// WithSpaceBlobReplicateMethod registers the space/blob/replicate handler.
// This is a stub implementation that acknowledges replication requests.
// TODO: Implement actual replication logic.
func WithSpaceBlobReplicateMethod(s SpaceBlobReplicateService) server.Option {
	return server.WithServiceMethod(
		spaceblobcap.ReplicateAbility,
		server.Provide(
			spaceblobcap.Replicate,
			func(ctx context.Context,
				cap ucan.Capability[spaceblobcap.ReplicateCaveats],
				inv invocation.Invocation,
				iCtx server.InvocationContext,
			) (result.Result[spaceblobcap.ReplicateOk, failure.IPLDBuilderFailure], fx.Effects, error) {

				spaceDID := cap.With()
				blob := cap.Nb().Blob
				replicas := cap.Nb().Replicas

				log.Printf("[space/blob/replicate] STUB: space=%s digest=%x replicas=%d (not implemented)",
					spaceDID, blob.Digest[:8], replicas)

				// Return empty success - guppy ignores Site promises
				return result.Ok[spaceblobcap.ReplicateOk, failure.IPLDBuilderFailure](
					spaceblobcap.ReplicateOk{},
				), nil, nil
			},
		),
	)
}
