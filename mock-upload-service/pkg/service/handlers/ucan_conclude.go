package handlers

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	blobcap "github.com/storacha/go-libstoracha/capabilities/blob"
	httpcap "github.com/storacha/go-libstoracha/capabilities/http"
	captypes "github.com/storacha/go-libstoracha/capabilities/types"
	ucancap "github.com/storacha/go-libstoracha/capabilities/ucan"
	"github.com/storacha/go-ucanto/core/dag/blockstore"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/ipld/block"
	"github.com/storacha/go-ucanto/core/receipt"
	"github.com/storacha/go-ucanto/core/receipt/fx"
	"github.com/storacha/go-ucanto/core/receipt/ran"
	"github.com/storacha/go-ucanto/core/result"
	"github.com/storacha/go-ucanto/core/result/failure"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/go-ucanto/principal"
	"github.com/storacha/go-ucanto/principal/ed25519/verifier"
	"github.com/storacha/go-ucanto/server"
	"github.com/storacha/go-ucanto/ucan"
	"github.com/storacha/go-ucanto/validator"

	"github.com/storacha/smelt/mock-upload-service/pkg/indexerclient"
	"github.com/storacha/smelt/mock-upload-service/pkg/piriclient"
	"github.com/storacha/smelt/mock-upload-service/pkg/state"
)

// UCANConcludeService defines the interface for the ucan/conclude handler.
type UCANConcludeService interface {
	ID() principal.Signer
	State() state.StateStore
	PiriClient(ctx context.Context) (*piriclient.Client, error)
	IndexerClient() *indexerclient.Client
}

// WithUCANConcludeMethod registers the ucan/conclude handler.
// This handler processes receipt conclusions from clients.
// When it receives an http/put receipt, it calls blob/accept on piri
// and stores the accept receipt for later retrieval.
func WithUCANConcludeMethod(s UCANConcludeService) server.Option {
	return server.WithServiceMethod(
		ucancap.ConcludeAbility,
		server.Provide(
			ucancap.Conclude,
			func(ctx context.Context,
				cap ucan.Capability[ucancap.ConcludeCaveats],
				inv invocation.Invocation,
				iCtx server.InvocationContext,
			) (result.Result[ucancap.ConcludeOk, failure.IPLDBuilderFailure], fx.Effects, error) {
				receiptLink := cap.Nb().Receipt

				log.Printf("[ucan/conclude] received receipt: %s", receiptLink.String())

				// Read the concluded receipt from the invocation's attached blocks
				anyReader := receipt.NewAnyReceiptReader(captypes.Converters...)
				rcpt, err := anyReader.Read(receiptLink, inv.Blocks())
				if err != nil {
					log.Printf("[ucan/conclude] failed to read concluded receipt: %v", err)
					// Still acknowledge even if we can't read the receipt
					return result.Ok[ucancap.ConcludeOk, failure.IPLDBuilderFailure](ucancap.ConcludeOk{}), nil, nil
				}

				// Get the invocation that the receipt is for
				ranInv, ok := rcpt.Ran().Invocation()
				if !ok {
					log.Printf("[ucan/conclude] receipt ran is not an invocation")
					return result.Ok[ucancap.ConcludeOk, failure.IPLDBuilderFailure](ucancap.ConcludeOk{}), nil, nil
				}

				// Check if this is an http/put receipt
				if len(ranInv.Capabilities()) == 0 {
					log.Printf("[ucan/conclude] invocation has no capabilities")
					return result.Ok[ucancap.ConcludeOk, failure.IPLDBuilderFailure](ucancap.ConcludeOk{}), nil, nil
				}

				ability := ranInv.Capabilities()[0].Can()
				log.Printf("[ucan/conclude] receipt is for ability: %s", ability)

				if ability != httpcap.PutAbility {
					// Not an http/put receipt, just acknowledge
					return result.Ok[ucancap.ConcludeOk, failure.IPLDBuilderFailure](ucancap.ConcludeOk{}), nil, nil
				}

				// This is an http/put receipt - we need to call blob/accept on piri
				log.Printf("[ucan/conclude] processing http/put receipt")

				// Extract the body (blob info) from the http/put invocation
				putCap := ranInv.Capabilities()[0]
				putMatch, err := httpcap.Put.Match(validator.NewSource(putCap, ranInv))
				if err != nil {
					log.Printf("[ucan/conclude] failed to match http/put capability: %v", err)
					return result.Ok[ucancap.ConcludeOk, failure.IPLDBuilderFailure](ucancap.ConcludeOk{}), nil, nil
				}

				body := putMatch.Value().Nb().Body
				digestHex := hex.EncodeToString(body.Digest)

				log.Printf("[ucan/conclude] http/put for blob digest=%s size=%d", digestHex[:16], body.Size)

				// Find the allocation for this blob
				alloc, err := s.State().GetAllocation(ctx, digestHex)
				if err != nil {
					log.Printf("[ucan/conclude] error getting allocation: %v", err)
					return result.Ok[ucancap.ConcludeOk, failure.IPLDBuilderFailure](ucancap.ConcludeOk{}), nil, nil
				}
				if alloc == nil {
					log.Printf("[ucan/conclude] allocation not found for digest: %s", digestHex[:16])
					return result.Ok[ucancap.ConcludeOk, failure.IPLDBuilderFailure](ucancap.ConcludeOk{}), nil, nil
				}

				// Get the piri client (queries provider table on each request)
				piriClient, err := s.PiriClient(ctx)
				if err != nil {
					log.Printf("[ucan/conclude] failed to get piri client: %v", err)
					return result.Ok[ucancap.ConcludeOk, failure.IPLDBuilderFailure](ucancap.ConcludeOk{}), nil, nil
				}
				if piriClient == nil {
					log.Printf("[ucan/conclude] no storage provider available")
					return result.Ok[ucancap.ConcludeOk, failure.IPLDBuilderFailure](ucancap.ConcludeOk{}), nil, nil
				}

				// Call blob/accept on piri
			// Use a detached context so the accept call doesn't get canceled
			// when the HTTP request context completes (e.g., during parallel uploads)
			log.Printf("[ucan/conclude] calling piri blob/accept")
			acceptCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			acceptResp, piriRcpt, err := piriClient.Accept(acceptCtx, &piriclient.AcceptRequest{
					Space:  alloc.Space,
					Digest: body.Digest,
					Size:   body.Size,
					Put:    ranInv.Link(),
				})
				if err != nil {
					log.Printf("[ucan/conclude] piri accept failed: %v", err)
					return result.Ok[ucancap.ConcludeOk, failure.IPLDBuilderFailure](ucancap.ConcludeOk{}), nil, nil
				}

				log.Printf("[ucan/conclude] piri accept succeeded, site=%v", acceptResp.Site)

				// Cache the location claim with the indexer
				if indexerClient := s.IndexerClient(); indexerClient != nil && acceptResp.Site != nil {
					// Extract the location claim delegation from piri's receipt blocks
					bs, bsErr := blockstore.NewBlockReader(blockstore.WithBlocksIterator(piriRcpt.Blocks()))
					if bsErr != nil {
						log.Printf("[ucan/conclude] failed to create blockstore from piri receipt: %v", bsErr)
					} else {
						locationClaim, claimErr := delegation.NewDelegationView(acceptResp.Site, bs)
						if claimErr != nil {
							log.Printf("[ucan/conclude] failed to read location claim delegation: %v", claimErr)
						} else {
							// Convert piri's DID to a libp2p peer ID for the multiaddr
							piriPeerID, peerIDErr := didToPeerID(piriClient.PiriDID())
							if peerIDErr != nil {
								log.Printf("[ucan/conclude] failed to convert piri DID to peer ID: %v", peerIDErr)
							} else {
								// Build piri's provider addresses for the indexer
								// Include BOTH {blobCID} and {claim} endpoints, just like piri does
								// The /p2p/ component tells the indexer the provider's peer ID
								piriBlobAddrStr := fmt.Sprintf("/dns4/piri/tcp/3000/http/p2p/%s/http-path/piece%%2F%%7BblobCID%%7D", piriPeerID.String())
								piriClaimAddrStr := fmt.Sprintf("/dns4/piri/tcp/3000/http/p2p/%s/http-path/claim%%2F%%7Bclaim%%7D", piriPeerID.String())
								piriBlobAddr, maErr1 := multiaddr.NewMultiaddr(piriBlobAddrStr)
								piriClaimAddr, maErr2 := multiaddr.NewMultiaddr(piriClaimAddrStr)
								if maErr1 != nil || maErr2 != nil {
									log.Printf("[ucan/conclude] failed to create piri multiaddr: blob=%v claim=%v", maErr1, maErr2)
								} else {
									log.Printf("[ucan/conclude] caching location claim with piri peer ID: %s (2 addresses)", piriPeerID.String())
									cacheErr := indexerClient.CacheLocationClaim(acceptCtx, locationClaim, []multiaddr.Multiaddr{piriBlobAddr, piriClaimAddr})
									if cacheErr != nil {
										log.Printf("[ucan/conclude] failed to cache location claim with indexer: %v", cacheErr)
										// Don't fail - indexing is best effort
									} else {
										log.Printf("[ucan/conclude] cached location claim with indexer for digest=%s", digestHex[:16])
									}
								}
							}
						}
					}
				}

				// Create a new receipt with the correct Ran reference
				// The piri receipt references piriclient's accept invocation, but guppy
				// is polling for the accept invocation from space/blob/add effects.
				// We re-issue the receipt with the correct Ran so guppy can find it.
				acceptOk := blobcap.AcceptOk{
					Site: acceptResp.Site,
				}
				reissuedRcpt, err := receipt.Issue(
					s.ID(),
					result.Ok[blobcap.AcceptOk, failure.IPLDBuilderFailure](acceptOk),
					ran.FromLink(alloc.AcceptInvLink),
				)
				if err != nil {
					log.Printf("[ucan/conclude] failed to re-issue receipt: %v", err)
					return result.Ok[ucancap.ConcludeOk, failure.IPLDBuilderFailure](ucancap.ConcludeOk{}), nil, nil
				}

				// Collect extra blocks from piri's receipt
				// This includes the location delegation blocks referenced by the Site link
				var extraBlocks []block.Block
				for blk, err := range piriRcpt.Blocks() {
					if err != nil {
						log.Printf("[ucan/conclude] error iterating piri receipt blocks: %v", err)
						continue
					}
					extraBlocks = append(extraBlocks, blk)
				}
				log.Printf("[ucan/conclude] collected %d extra blocks from piri receipt", len(extraBlocks))

				acceptInvLink := alloc.AcceptInvLink.String()
				log.Printf("[ucan/conclude] re-issued receipt for task: %s", acceptInvLink)

				if err := s.State().PutReceipt(ctx, acceptInvLink, &state.StoredReceipt{
					Task:        alloc.AcceptInvLink,
					Receipt:     reissuedRcpt,
					ExtraBlocks: extraBlocks,
					AddedAt:     time.Now(),
				}); err != nil {
					log.Printf("[ucan/conclude] failed to store receipt: %v", err)
					return result.Ok[ucancap.ConcludeOk, failure.IPLDBuilderFailure](ucancap.ConcludeOk{}), nil, nil
				}

				log.Printf("[ucan/conclude] stored accept receipt for task: %s", acceptInvLink)

				return result.Ok[ucancap.ConcludeOk, failure.IPLDBuilderFailure](ucancap.ConcludeOk{}), nil, nil
			},
		),
	)
}

// didToPeerID converts a did:key DID to a libp2p peer ID.
// This only works for ed25519 DIDs (did:key:z6Mk...).
func didToPeerID(d did.DID) (peer.ID, error) {
	vfr, err := verifier.Decode(d.Bytes())
	if err != nil {
		return "", fmt.Errorf("decoding DID to verifier: %w", err)
	}
	pub, err := crypto.UnmarshalEd25519PublicKey(vfr.Raw())
	if err != nil {
		return "", fmt.Errorf("unmarshaling ed25519 public key: %w", err)
	}
	return peer.IDFromPublicKey(pub)
}
