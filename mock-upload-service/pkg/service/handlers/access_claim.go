package handlers

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipld/go-ipld-prime/node/basicnode"
	"github.com/storacha/go-libstoracha/capabilities/access"
	ucancap "github.com/storacha/go-libstoracha/capabilities/ucan"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/ipld"
	"github.com/storacha/go-ucanto/core/receipt/fx"
	"github.com/storacha/go-ucanto/core/result"
	"github.com/storacha/go-ucanto/core/result/failure"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/go-ucanto/principal"
	"github.com/storacha/go-ucanto/principal/absentee"
	"github.com/storacha/go-ucanto/server"
	"github.com/storacha/go-ucanto/ucan"

	"github.com/storacha/smelt/mock-upload-service/pkg/state"
)

// AccessClaimService defines the interface for the access/claim handler.
type AccessClaimService interface {
	ID() principal.Signer
	State() state.StateStore
}

// WithAccessClaimMethod registers the access/claim handler.
// This handler returns delegations for any pending auth requests.
func WithAccessClaimMethod(s AccessClaimService) server.Option {
	return server.WithServiceMethod(
		access.ClaimAbility,
		server.Provide(
			access.Claim,
			func(ctx context.Context,
				cap ucan.Capability[access.ClaimCaveats],
				inv invocation.Invocation,
				iCtx server.InvocationContext,
			) (result.Result[access.ClaimOk, failure.IPLDBuilderFailure], fx.Effects, error) {

				agentDID := inv.Issuer().DID().String()
				log.Printf("[access/claim] agent=%s", agentDID)

				// Find pending auth requests for this agent
				requests, err := s.State().GetAuthRequestsByAgent(ctx, agentDID)
				if err != nil {
					log.Printf("[access/claim] failed to get auth requests: %v", err)
					return nil, nil, fmt.Errorf("getting auth requests: %w", err)
				}
				log.Printf("[access/claim] found %d pending requests", len(requests))

				delegationsMap := make(map[string][]byte)
				var keys []string

				for _, req := range requests {
					// Create delegation from account (using absentee signer) to agent with full access
					accountDel, err := createAccountDelegation(agentDID, req.AccountDID, req.RequestLink)
					if err != nil {
						return nil, nil, fmt.Errorf("creating account delegation: %w", err)
					}

					// Create ucan/attest session proof from service attesting to the delegation
					sessionProof, err := createSessionProof(s.ID(), agentDID, accountDel.Link())
					if err != nil {
						return nil, nil, fmt.Errorf("creating session proof: %w", err)
					}

					// Archive account delegation to bytes
					accountArchiveReader := accountDel.Archive()
					accountArchive, err := io.ReadAll(accountArchiveReader)
					if err != nil {
						return nil, nil, fmt.Errorf("reading account delegation archive: %w", err)
					}

					// Archive session proof to bytes
					sessionArchiveReader := sessionProof.Archive()
					sessionArchive, err := io.ReadAll(sessionArchiveReader)
					if err != nil {
						return nil, nil, fmt.Errorf("reading session proof archive: %w", err)
					}

					// Add account delegation
					accountLinkStr := accountDel.Link().String()
					delegationsMap[accountLinkStr] = accountArchive
					keys = append(keys, accountLinkStr)
					log.Printf("[access/claim] created account delegation %s for account %s (size=%d)", accountLinkStr, req.AccountDID, len(accountArchive))

					// Add session proof
					sessionLinkStr := sessionProof.Link().String()
					delegationsMap[sessionLinkStr] = sessionArchive
					keys = append(keys, sessionLinkStr)
					log.Printf("[access/claim] created session proof %s attesting to %s (size=%d)", sessionLinkStr, accountLinkStr, len(sessionArchive))

					// Mark as claimed
					if err := s.State().MarkAuthRequestClaimed(ctx, req.RequestLink); err != nil {
						log.Printf("[access/claim] failed to mark auth request claimed: %v", err)
					}
				}

				return result.Ok[access.ClaimOk, failure.IPLDBuilderFailure](access.ClaimOk{
					Delegations: access.DelegationsModel{
						Keys:   keys,
						Values: delegationsMap,
					},
				}), nil, nil
			},
		),
	)
}

// accessRequestFact implements ucan.FactBuilder for the access/request fact.
type accessRequestFact struct {
	requestLink cid.Cid
}

func (f accessRequestFact) ToIPLD() (map[string]datamodel.Node, error) {
	link := cidlink.Link{Cid: f.requestLink}
	return map[string]datamodel.Node{
		access.AuthorizeRequestFactKey: basicnode.NewLink(link),
	}, nil
}

// createAccountDelegation creates a delegation from the account to the agent.
// The account uses an absentee signer (since did:mailto can't actually sign).
// The requestLink is added as a fact so clients can verify this delegation was created for
// a specific access/authorize request.
func createAccountDelegation(agentDID, accountDID, requestLink string) (delegation.Delegation, error) {
	agent, err := did.Parse(agentDID)
	if err != nil {
		return nil, fmt.Errorf("parsing agent DID: %w", err)
	}

	account, err := did.Parse(accountDID)
	if err != nil {
		return nil, fmt.Errorf("parsing account DID: %w", err)
	}

	// Parse the request link as a CID
	requestCID, err := cid.Parse(requestLink)
	if err != nil {
		return nil, fmt.Errorf("parsing request link: %w", err)
	}

	// Create an absentee signer for the account (since did:mailto can't sign)
	accountSigner := absentee.From(account)

	// Grant full universal access (ucan:* means any resource)
	// This allows the agent to act on behalf of the account for all operations
	capabilities := []ucan.Capability[ucan.NoCaveats]{
		ucan.NewCapability("*", "ucan:*", ucan.NoCaveats{}),
	}

	// Add fact with the access/request link
	facts := []ucan.FactBuilder{
		accessRequestFact{requestLink: requestCID},
	}

	return delegation.Delegate(
		accountSigner,
		agent,
		capabilities,
		delegation.WithNoExpiration(),
		delegation.WithFacts(facts),
	)
}

// createSessionProof creates a ucan/attest delegation that attests to the account delegation.
// This proves the service has verified the account authorized the agent.
func createSessionProof(serviceSigner principal.Signer, agentDID string, proofLink ipld.Link) (delegation.Delegation, error) {
	agent, err := did.Parse(agentDID)
	if err != nil {
		return nil, fmt.Errorf("parsing agent DID: %w", err)
	}

	// Create ucan/attest capability
	return ucancap.Attest.Delegate(
		serviceSigner,
		agent,
		serviceSigner.DID().String(),
		ucancap.AttestCaveats{Proof: proofLink},
		delegation.WithNoExpiration(),
	)
}
