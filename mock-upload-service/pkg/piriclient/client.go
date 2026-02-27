package piriclient

import (
	"context"
	"fmt"
	"log"
	"net/url"

	blobcap "github.com/storacha/go-libstoracha/capabilities/blob"
	captypes "github.com/storacha/go-libstoracha/capabilities/types"
	uclient "github.com/storacha/go-ucanto/client"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/ipld"
	"github.com/storacha/go-ucanto/core/receipt"
	"github.com/storacha/go-ucanto/core/result"
	fdm "github.com/storacha/go-ucanto/core/result/failure/datamodel"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/go-ucanto/principal"
	ucanhttp "github.com/storacha/go-ucanto/transport/http"
)

// DelegationFetcher provides an interface for fetching delegation proofs on-demand.
type DelegationFetcher interface {
	// GetDelegation fetches the delegation proof for the given provider DID.
	// Returns nil if no delegation is available (not an error condition).
	GetDelegation(ctx context.Context, providerDID string) (delegation.Delegation, error)
}

// Client is a UCAN client for communicating with Piri nodes.
type Client struct {
	endpoint          *url.URL
	piriDID           did.DID
	signer            principal.Signer
	connection        uclient.Connection
	delegationFetcher DelegationFetcher
}

// New creates a new Piri client.
// The delegationFetcher is used to fetch delegation proofs on-demand for each request.
func New(endpoint *url.URL, piriDID did.DID, signer principal.Signer, fetcher DelegationFetcher) (*Client, error) {
	channel := ucanhttp.NewChannel(endpoint)
	conn, err := uclient.NewConnection(piriDID, channel)
	if err != nil {
		return nil, fmt.Errorf("creating connection: %w", err)
	}

	return &Client{
		endpoint:          endpoint,
		piriDID:           piriDID,
		signer:            signer,
		connection:        conn,
		delegationFetcher: fetcher,
	}, nil
}

// AllocateRequest contains the parameters for a blob/allocate invocation.
type AllocateRequest struct {
	Space  did.DID
	Digest []byte
	Size   uint64
	Cause  ipld.Link
}

// AllocateResponse contains the response from a blob/allocate invocation.
type AllocateResponse struct {
	Size    uint64
	Address *blobcap.Address
}

// fetchDelegationOpts fetches the delegation proof and returns delegation options.
func (c *Client) fetchDelegationOpts(ctx context.Context) ([]delegation.Option, error) {
	var opts []delegation.Option

	if c.delegationFetcher != nil {
		log.Printf("[piriclient] fetching delegation for piri DID: %s", c.piriDID.String())
		proof, err := c.delegationFetcher.GetDelegation(ctx, c.piriDID.String())
		if err != nil {
			log.Printf("[piriclient] delegation fetch error: %v", err)
			return nil, fmt.Errorf("fetching delegation: %w", err)
		}
		if proof != nil {
			log.Printf("[piriclient] found delegation: issuer=%s audience=%s", proof.Issuer().DID().String(), proof.Audience().DID().String())
			opts = append(opts, delegation.WithProof(delegation.FromDelegation(proof)))
		} else {
			log.Printf("[piriclient] no delegation found for piri DID: %s", c.piriDID.String())
		}
	} else {
		log.Printf("[piriclient] no delegation fetcher configured")
	}

	return opts, nil
}

// Allocate sends a blob/allocate invocation to the piri node.
// Returns the response data, the invocation that was sent, and the receipt from piri.
func (c *Client) Allocate(ctx context.Context, req *AllocateRequest) (*AllocateResponse, invocation.Invocation, receipt.AnyReceipt, error) {
	// Fetch delegation fresh for each request
	opts, err := c.fetchDelegationOpts(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	// Create the invocation
	// The resource (With) must be the piri node's DID for blob/allocate
	inv, err := blobcap.Allocate.Invoke(
		c.signer,
		c.piriDID,
		c.piriDID.String(), // resource is the piri DID
		blobcap.AllocateCaveats{
			Space: req.Space,
			Blob: captypes.Blob{
				Digest: req.Digest,
				Size:   req.Size,
			},
			Cause: req.Cause,
		},
		opts...,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("creating allocate invocation: %w", err)
	}

	// DEBUG: Log invocation chain to trace did:mailto leak
	log.Printf("[piriclient] ALLOCATE invocation created:")
	log.Printf("[piriclient]   Issuer: %s", inv.Issuer().DID().String())
	log.Printf("[piriclient]   Audience: %s", inv.Audience().DID().String())
	proofLinks := inv.Proofs()
	log.Printf("[piriclient]   Proof links count: %d", len(proofLinks))
	for i, prfLink := range proofLinks {
		log.Printf("[piriclient]   Proof[%d] link: %s", i, prfLink.String())
	}
	// Log all exported blocks
	blockCount := 0
	for blk, blkErr := range inv.Export() {
		if blkErr != nil {
			continue
		}
		log.Printf("[piriclient]   Block[%d]: %s", blockCount, blk.Link().String())
		blockCount++
	}
	log.Printf("[piriclient]   Total blocks: %d", blockCount)

	// Execute the invocation
	resp, err := uclient.Execute(ctx, []invocation.Invocation{inv}, c.connection)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("executing allocate invocation: %w", err)
	}

	// Get the receipt
	rcptLink, ok := resp.Get(inv.Link())
	if !ok {
		return nil, nil, nil, fmt.Errorf("receipt not found for invocation")
	}

	// Read the receipt using the any reader to avoid type issues
	anyReader := receipt.NewAnyReceiptReader(captypes.Converters...)
	anyRcpt, err := anyReader.Read(rcptLink, resp.Blocks())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading receipt: %w", err)
	}

	// Check for error response
	okNode, errNode := result.Unwrap(anyRcpt.Out())
	if errNode != nil {
		// Try to extract error details
		var errDetails string
		if msgNode, lookupErr := errNode.LookupByString("message"); lookupErr == nil {
			if msg, asErr := msgNode.AsString(); asErr == nil {
				errDetails = msg
			}
		}
		if errDetails == "" {
			if nameNode, lookupErr := errNode.LookupByString("name"); lookupErr == nil {
				if name, asErr := nameNode.AsString(); asErr == nil {
					errDetails = name
				}
			}
		}
		if errDetails == "" {
			errDetails = "unknown error"
		}
		return nil, nil, nil, fmt.Errorf("allocate failed: %s", errDetails)
	}
	if okNode == nil {
		return nil, nil, nil, fmt.Errorf("allocate returned nil result")
	}

	// Rebind to the typed receipt
	typedRcpt, err := receipt.Rebind[blobcap.AllocateOk, fdm.FailureModel](
		anyRcpt,
		blobcap.AllocateOkType(),
		fdm.FailureType(),
		captypes.Converters...,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("rebinding receipt: %w", err)
	}

	// Extract the result
	allocateOk, failErr := result.Unwrap(typedRcpt.Out())
	if (failErr != fdm.FailureModel{}) {
		return nil, nil, nil, fmt.Errorf("allocate failed: %s", failErr.Message)
	}

	return &AllocateResponse{
		Size:    allocateOk.Size,
		Address: allocateOk.Address,
	}, inv, anyRcpt, nil
}

// AllocateInvocation returns the invocation for the allocate request (for use in effects).
func (c *Client) AllocateInvocation(ctx context.Context, req *AllocateRequest) (invocation.IssuedInvocation, error) {
	opts, err := c.fetchDelegationOpts(ctx)
	if err != nil {
		return nil, err
	}

	return blobcap.Allocate.Invoke(
		c.signer,
		c.piriDID,
		c.piriDID.String(),
		blobcap.AllocateCaveats{
			Space: req.Space,
			Blob: captypes.Blob{
				Digest: req.Digest,
				Size:   req.Size,
			},
			Cause: req.Cause,
		},
		opts...,
	)
}

// PiriDID returns the DID of the piri node.
func (c *Client) PiriDID() did.DID {
	return c.piriDID
}

// AcceptRequest contains the parameters for a blob/accept invocation.
type AcceptRequest struct {
	Space  did.DID
	Digest []byte
	Size   uint64
	Put    ipld.Link // Link to the http/put invocation that uploaded the blob
}

// AcceptResponse contains the response from a blob/accept invocation.
type AcceptResponse struct {
	Site ipld.Link // Link to the location claim delegation
}

// Accept sends a blob/accept invocation to the piri node.
func (c *Client) Accept(ctx context.Context, req *AcceptRequest) (*AcceptResponse, receipt.AnyReceipt, error) {
	// Fetch delegation fresh for each request
	opts, err := c.fetchDelegationOpts(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Use WithNoExpiration so the invocation CID is deterministic and matches
	// the accept invocation created in space/blob/add for effects
	opts = append(opts, delegation.WithNoExpiration())
	inv, err := blobcap.Accept.Invoke(
		c.signer,
		c.piriDID,
		c.piriDID.String(),
		blobcap.AcceptCaveats{
			Space: req.Space,
			Blob: captypes.Blob{
				Digest: req.Digest,
				Size:   req.Size,
			},
			Put: blobcap.Promise{
				UcanAwait: blobcap.Await{
					Selector: ".out.ok",
					Link:     req.Put,
				},
			},
		},
		opts...,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("creating accept invocation: %w", err)
	}

	// DEBUG: Log invocation chain to trace did:mailto leak
	log.Printf("[piriclient] ACCEPT invocation created:")
	log.Printf("[piriclient]   Issuer: %s", inv.Issuer().DID().String())
	log.Printf("[piriclient]   Audience: %s", inv.Audience().DID().String())
	acceptProofLinks := inv.Proofs()
	log.Printf("[piriclient]   Proof links count: %d", len(acceptProofLinks))
	for i, prfLink := range acceptProofLinks {
		log.Printf("[piriclient]   Proof[%d] link: %s", i, prfLink.String())
	}
	// Log all exported blocks
	acceptBlockCount := 0
	for blk, blkErr := range inv.Export() {
		if blkErr != nil {
			continue
		}
		log.Printf("[piriclient]   Block[%d]: %s", acceptBlockCount, blk.Link().String())
		acceptBlockCount++
	}
	log.Printf("[piriclient]   Total blocks: %d", acceptBlockCount)

	// Execute the invocation
	resp, err := uclient.Execute(ctx, []invocation.Invocation{inv}, c.connection)
	if err != nil {
		return nil, nil, fmt.Errorf("executing accept invocation: %w", err)
	}

	// Get the receipt
	rcptLink, ok := resp.Get(inv.Link())
	if !ok {
		return nil, nil, fmt.Errorf("receipt not found for invocation")
	}

	// Read the receipt using the any reader
	anyReader := receipt.NewAnyReceiptReader(captypes.Converters...)
	anyRcpt, err := anyReader.Read(rcptLink, resp.Blocks())
	if err != nil {
		return nil, nil, fmt.Errorf("reading receipt: %w", err)
	}

	// Check for error response
	okNode, errNode := result.Unwrap(anyRcpt.Out())
	if errNode != nil {
		var errDetails string
		if msgNode, lookupErr := errNode.LookupByString("message"); lookupErr == nil {
			if msg, asErr := msgNode.AsString(); asErr == nil {
				errDetails = msg
			}
		}
		if errDetails == "" {
			if nameNode, lookupErr := errNode.LookupByString("name"); lookupErr == nil {
				if name, asErr := nameNode.AsString(); asErr == nil {
					errDetails = name
				}
			}
		}
		if errDetails == "" {
			errDetails = "unknown error"
		}
		return nil, nil, fmt.Errorf("accept failed: %s", errDetails)
	}
	if okNode == nil {
		return nil, nil, fmt.Errorf("accept returned nil result")
	}

	// Extract the site link from the ok node
	var site ipld.Link
	if siteNode, lookupErr := okNode.LookupByString("site"); lookupErr == nil {
		if siteLink, asErr := siteNode.AsLink(); asErr == nil {
			site = siteLink
		}
	}

	return &AcceptResponse{
		Site: site,
	}, anyRcpt, nil
}
