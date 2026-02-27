package handlers

import (
	"context"

	"github.com/storacha/go-libstoracha/capabilities/provider"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/receipt/fx"
	"github.com/storacha/go-ucanto/core/result"
	"github.com/storacha/go-ucanto/core/result/failure"
	"github.com/storacha/go-ucanto/principal"
	"github.com/storacha/go-ucanto/server"
	"github.com/storacha/go-ucanto/ucan"

	"github.com/storacha/smelt/mock-upload-service/pkg/state"
)

// ProviderAddService defines the interface for the provider/add handler.
type ProviderAddService interface {
	ID() principal.Signer
	State() state.StateStore
}

// WithProviderAddMethod registers the provider/add handler.
// This handler provisions a space to an account.
func WithProviderAddMethod(s ProviderAddService) server.Option {
	return server.WithServiceMethod(
		provider.AddAbility,
		server.Provide(
			provider.Add,
			func(ctx context.Context,
				cap ucan.Capability[provider.AddCaveats],
				inv invocation.Invocation,
				iCtx server.InvocationContext,
			) (result.Result[provider.AddOk, failure.IPLDBuilderFailure], fx.Effects, error) {

				// cap.With() = account DID (did:mailto:...)
				// cap.Nb().Provider = upload service DID
				// cap.Nb().Consumer = space DID

				// Store the provisioning
				if err := s.State().PutProvisioning(ctx, cap.Nb().Consumer, &state.Provisioning{
					Account:  cap.With(),
					Provider: cap.Nb().Provider,
					Space:    cap.Nb().Consumer,
				}); err != nil {
					return result.Error[provider.AddOk, failure.IPLDBuilderFailure](
						failure.FromError(err),
					), nil, nil
				}

				// Return success with space DID as ID
				return result.Ok[provider.AddOk, failure.IPLDBuilderFailure](provider.AddOk{
					Id: cap.Nb().Consumer,
				}), nil, nil
			},
		),
	)
}
