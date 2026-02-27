package handlers

import (
	"context"
	"log"
	"time"

	"github.com/storacha/go-libstoracha/capabilities/access"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/receipt/fx"
	"github.com/storacha/go-ucanto/core/result"
	"github.com/storacha/go-ucanto/core/result/failure"
	"github.com/storacha/go-ucanto/principal"
	"github.com/storacha/go-ucanto/server"
	"github.com/storacha/go-ucanto/ucan"

	"github.com/storacha/smelt/mock-upload-service/pkg/state"
)

// AccessAuthorizeService defines the interface for the access/authorize handler.
type AccessAuthorizeService interface {
	ID() principal.Signer
	State() state.StateStore
}

// WithAccessAuthorizeMethod registers the access/authorize handler.
// This handler auto-approves login requests immediately (no email verification).
func WithAccessAuthorizeMethod(s AccessAuthorizeService) server.Option {
	return server.WithServiceMethod(
		access.AuthorizeAbility,
		server.Provide(
			access.Authorize,
			func(ctx context.Context,
				cap ucan.Capability[access.AuthorizeCaveats],
				inv invocation.Invocation,
				iCtx server.InvocationContext,
			) (result.Result[access.AuthorizeOk, failure.IPLDBuilderFailure], fx.Effects, error) {

				// Get the account DID from the caveats
				accountDID := ""
				if cap.Nb().Iss != nil {
					accountDID = *cap.Nb().Iss
				}

				agentDID := inv.Issuer().DID().String()
				log.Printf("[access/authorize] agent=%s account=%s", agentDID, accountDID)

				// Store the auth request for later claiming
				if err := s.State().PutAuthRequest(ctx, inv.Link().String(), &state.AuthRequest{
					AgentDID:    agentDID,
					AccountDID:  accountDID,
					RequestLink: inv.Link().String(),
					Expiration:  time.Now().Add(24 * time.Hour),
					Claimed:     false,
				}); err != nil {
					log.Printf("[access/authorize] failed to store auth request: %v", err)
					return result.Error[access.AuthorizeOk, failure.IPLDBuilderFailure](
						failure.FromError(err),
					), nil, nil
				}

				// Return success immediately (auto-approve for local dev)
				expiration := ucan.UTCUnixTimestamp(time.Now().Add(24 * time.Hour).Unix())
				return result.Ok[access.AuthorizeOk, failure.IPLDBuilderFailure](access.AuthorizeOk{
					Request:    inv.Link(),
					Expiration: expiration,
				}), nil, nil
			},
		),
	)
}
