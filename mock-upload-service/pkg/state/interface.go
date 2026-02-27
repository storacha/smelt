package state

import (
	"context"
	"net/url"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/ipld/block"
	"github.com/storacha/go-ucanto/core/receipt"
	"github.com/storacha/go-ucanto/did"
)

// StateStore defines the interface for state storage operations.
// All methods take a context for timeout/cancellation support.
type StateStore interface {
	// Allocations
	PutAllocation(ctx context.Context, key string, alloc *Allocation) error
	GetAllocation(ctx context.Context, key string) (*Allocation, error)
	DeleteAllocation(ctx context.Context, key string) error

	// Receipts
	PutReceipt(ctx context.Context, taskCID string, rcpt *StoredReceipt) error
	GetReceipt(ctx context.Context, taskCID string) (*StoredReceipt, error)

	// Auth Requests
	PutAuthRequest(ctx context.Context, linkCID string, req *AuthRequest) error
	GetAuthRequestsByAgent(ctx context.Context, agentDID string) ([]*AuthRequest, error)
	MarkAuthRequestClaimed(ctx context.Context, linkCID string) error

	// Provisionings
	PutProvisioning(ctx context.Context, spaceDID string, prov *Provisioning) error
	GetProvisioning(ctx context.Context, spaceDID string) (*Provisioning, error)

	// Uploads
	PutUpload(ctx context.Context, spaceDID string, upload *Upload) error
	GetUploads(ctx context.Context, spaceDID string) ([]*Upload, error)

	// Providers
	GetFirstProvider(ctx context.Context) (*Provider, error)

	// Delegations (for piriclient.DelegationFetcher interface)
	GetDelegation(ctx context.Context, providerDID string) (delegation.Delegation, error)
}

// Allocation represents a pending blob upload allocation.
type Allocation struct {
	Space         did.DID
	Digest        multihash.Multihash
	Size          uint64
	Cause         ipld.Link // Link to the allocation invocation
	ExpiresAt     time.Time
	PiriNode      string    // Which Piri node handles this
	UploadURL     *url.URL  // Presigned URL from Piri
	AcceptInvLink ipld.Link // Link to the accept invocation returned to guppy
}

// Acceptance represents a completed blob upload.
type Acceptance struct {
	Space       did.DID
	Digest      multihash.Multihash
	Size        uint64
	Cause       ipld.Link
	LocationCID cid.Cid
	AcceptedAt  time.Time
}

// Upload represents a registered upload (root + shards mapping).
type Upload struct {
	Space   did.DID
	Root    ipld.Link
	Shards  []ipld.Link
	AddedAt time.Time
}

// StoredReceipt holds a UCAN receipt for later retrieval.
type StoredReceipt struct {
	Task        ipld.Link
	Receipt     receipt.AnyReceipt
	ExtraBlocks []block.Block // Additional blocks to include (e.g., location delegation from piri)
	AddedAt     time.Time
}

// Provider represents a registered Piri storage node.
type Provider struct {
	DID      did.DID
	Endpoint *url.URL
	Weight   int // For future multi-node selection
}

// AuthRequest represents a pending authorization request (login flow).
type AuthRequest struct {
	AgentDID    string    // did:key of the requesting agent
	AccountDID  string    // did:mailto account they want to act as
	RequestLink string    // CID link to the original request
	Expiration  time.Time // When this request expires
	Claimed     bool      // Whether delegations have been claimed
}

// Provisioning represents a space provisioned to an account.
type Provisioning struct {
	Account  string // did:mailto account
	Provider string // upload service DID
	Space    string // space DID
}
