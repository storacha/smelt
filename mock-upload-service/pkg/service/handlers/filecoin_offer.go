package handlers

import (
	"context"
	"log"

	filecoincap "github.com/storacha/go-libstoracha/capabilities/filecoin"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/receipt/fx"
	"github.com/storacha/go-ucanto/core/result"
	"github.com/storacha/go-ucanto/core/result/failure"
	"github.com/storacha/go-ucanto/principal"
	"github.com/storacha/go-ucanto/server"
	"github.com/storacha/go-ucanto/ucan"
)

// FilecoinOfferService defines the interface for the filecoin/offer handler.
type FilecoinOfferService interface {
	ID() principal.Signer
}

// WithFilecoinOfferMethod registers the filecoin/offer handler.
// This is a stub implementation that acknowledges Filecoin storage offers.
// TODO: Implement actual Filecoin deal making.
func WithFilecoinOfferMethod(s FilecoinOfferService) server.Option {
	return server.WithServiceMethod(
		filecoincap.OfferAbility,
		server.Provide(
			filecoincap.Offer,
			func(ctx context.Context,
				cap ucan.Capability[filecoincap.OfferCaveats],
				inv invocation.Invocation,
				iCtx server.InvocationContext,
			) (result.Result[filecoincap.OfferOk, failure.IPLDBuilderFailure], fx.Effects, error) {

				spaceDID := cap.With()
				content := cap.Nb().Content
				piece := cap.Nb().Piece

				log.Printf("[filecoin/offer] STUB: space=%s content=%s piece=%s (not implemented)",
					spaceDID, content.String(), piece.String())

				// Return success echoing back the piece CID
				return result.Ok[filecoincap.OfferOk, failure.IPLDBuilderFailure](
					filecoincap.OfferOk{
						Piece: piece,
					},
				), nil, nil
			},
		),
	)
}
