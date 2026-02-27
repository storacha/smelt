package state

import (
	"context"
	"sync"

	"github.com/storacha/go-ucanto/core/delegation"
)

// Store holds all in-memory state for the mock service.
// This is a simple implementation for testing; production would use DynamoDB.
type Store struct {
	mu sync.RWMutex

	allocations   map[string]*Allocation     // key: multihash hex
	acceptances   map[string]*Acceptance     // key: multihash hex
	uploads       map[string][]*Upload       // key: space DID string
	receipts      map[string]*StoredReceipt  // key: task CID string
	providers     map[string]*Provider       // key: provider DID string
	authRequests  map[string]*AuthRequest    // key: request link CID
	provisionings map[string]*Provisioning   // key: space DID
}

// Ensure Store implements StateStore interface
var _ StateStore = (*Store)(nil)

// NewStore creates a new in-memory state store.
func NewStore() *Store {
	return &Store{
		allocations:   make(map[string]*Allocation),
		acceptances:   make(map[string]*Acceptance),
		uploads:       make(map[string][]*Upload),
		receipts:      make(map[string]*StoredReceipt),
		providers:     make(map[string]*Provider),
		authRequests:  make(map[string]*AuthRequest),
		provisionings: make(map[string]*Provisioning),
	}
}

// PutAllocation stores an allocation.
func (s *Store) PutAllocation(ctx context.Context, key string, alloc *Allocation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allocations[key] = alloc
	return nil
}

// GetAllocation retrieves an allocation by key.
func (s *Store) GetAllocation(ctx context.Context, key string) (*Allocation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	alloc, ok := s.allocations[key]
	if !ok {
		return nil, nil
	}
	return alloc, nil
}

// DeleteAllocation removes an allocation.
func (s *Store) DeleteAllocation(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.allocations, key)
	return nil
}

// PutAcceptance stores an acceptance record.
func (s *Store) PutAcceptance(ctx context.Context, key string, acc *Acceptance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acceptances[key] = acc
	return nil
}

// GetAcceptance retrieves an acceptance by key.
func (s *Store) GetAcceptance(ctx context.Context, key string) (*Acceptance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	acc, ok := s.acceptances[key]
	if !ok {
		return nil, nil
	}
	return acc, nil
}

// PutUpload stores an upload record.
func (s *Store) PutUpload(ctx context.Context, spaceKey string, upload *Upload) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uploads[spaceKey] = append(s.uploads[spaceKey], upload)
	return nil
}

// GetUploads retrieves all uploads for a space.
func (s *Store) GetUploads(ctx context.Context, spaceKey string) ([]*Upload, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.uploads[spaceKey], nil
}

// PutReceipt stores a receipt.
func (s *Store) PutReceipt(ctx context.Context, taskCID string, rcpt *StoredReceipt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.receipts[taskCID] = rcpt
	return nil
}

// GetReceipt retrieves a receipt by task CID.
func (s *Store) GetReceipt(ctx context.Context, taskCID string) (*StoredReceipt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rcpt, ok := s.receipts[taskCID]
	if !ok {
		return nil, nil
	}
	return rcpt, nil
}

// PutProvider registers a storage provider.
func (s *Store) PutProvider(ctx context.Context, didKey string, provider *Provider) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers[didKey] = provider
	return nil
}

// GetProvider retrieves a provider by DID.
func (s *Store) GetProvider(ctx context.Context, didKey string) (*Provider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	provider, ok := s.providers[didKey]
	if !ok {
		return nil, nil
	}
	return provider, nil
}

// GetAllProviders returns all registered providers.
func (s *Store) GetAllProviders(ctx context.Context) ([]*Provider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	providers := make([]*Provider, 0, len(s.providers))
	for _, p := range s.providers {
		providers = append(providers, p)
	}
	return providers, nil
}

// GetFirstProvider returns the first available provider (for simple routing).
func (s *Store) GetFirstProvider(ctx context.Context) (*Provider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.providers {
		return p, nil
	}
	return nil, nil
}

// PutAuthRequest stores an authorization request.
func (s *Store) PutAuthRequest(ctx context.Context, linkCID string, req *AuthRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authRequests[linkCID] = req
	return nil
}

// GetAuthRequest retrieves an authorization request by link CID.
func (s *Store) GetAuthRequest(ctx context.Context, linkCID string) (*AuthRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	req, ok := s.authRequests[linkCID]
	if !ok {
		return nil, nil
	}
	return req, nil
}

// GetAuthRequestsByAgent retrieves all pending auth requests for an agent.
func (s *Store) GetAuthRequestsByAgent(ctx context.Context, agentDID string) ([]*AuthRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var requests []*AuthRequest
	for _, req := range s.authRequests {
		if req.AgentDID == agentDID && !req.Claimed {
			requests = append(requests, req)
		}
	}
	return requests, nil
}

// MarkAuthRequestClaimed marks an auth request as claimed.
func (s *Store) MarkAuthRequestClaimed(ctx context.Context, linkCID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req, ok := s.authRequests[linkCID]; ok {
		req.Claimed = true
	}
	return nil
}

// PutProvisioning stores a space provisioning record.
func (s *Store) PutProvisioning(ctx context.Context, spaceDID string, prov *Provisioning) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.provisionings[spaceDID] = prov
	return nil
}

// GetProvisioning retrieves a provisioning by space DID.
func (s *Store) GetProvisioning(ctx context.Context, spaceDID string) (*Provisioning, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	prov, ok := s.provisionings[spaceDID]
	if !ok {
		return nil, nil
	}
	return prov, nil
}

// GetDelegation returns the delegation for a provider DID.
// The in-memory store doesn't store delegations, so this always returns nil.
func (s *Store) GetDelegation(ctx context.Context, providerDID string) (delegation.Delegation, error) {
	// In-memory store doesn't track delegations
	return nil, nil
}
