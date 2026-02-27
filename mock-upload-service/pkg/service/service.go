package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"iter"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/ipld"
	"github.com/storacha/go-ucanto/core/ipld/block"
	"github.com/storacha/go-ucanto/core/message"
	"github.com/storacha/go-ucanto/core/receipt"
	"github.com/storacha/go-ucanto/core/receipt/fx"
	"github.com/storacha/go-ucanto/core/receipt/ran"
	"github.com/storacha/go-ucanto/core/result"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/go-ucanto/principal"
	"github.com/storacha/go-ucanto/server"
	"github.com/storacha/go-ucanto/transport/car"
	ucanhttp "github.com/storacha/go-ucanto/transport/http"
	"github.com/storacha/go-ucanto/ucan"
	"github.com/storacha/go-ucanto/ucan/crypto/signature"
	"go.uber.org/zap"

	"github.com/storacha/smelt/mock-upload-service/pkg/config"
	"github.com/storacha/smelt/mock-upload-service/pkg/identity"
	"github.com/storacha/smelt/mock-upload-service/pkg/indexerclient"
	"github.com/storacha/smelt/mock-upload-service/pkg/piriclient"
	"github.com/storacha/smelt/mock-upload-service/pkg/service/handlers"
	"github.com/storacha/smelt/mock-upload-service/pkg/state"
)

// Service implements the mock upload service logic.
type Service struct {
	cfg           *config.Config
	identity      *identity.Identity
	state         state.StateStore
	indexerClient *indexerclient.Client
	logger        *zap.Logger
	ucanServer    server.ServerView[server.Service]
}

// New creates a new Service instance.
func New(cfg *config.Config, id *identity.Identity, store state.StateStore, indexerClient *indexerclient.Client, logger *zap.Logger) (*Service, error) {
	svc := &Service{
		cfg:           cfg,
		identity:      id,
		state:         store,
		indexerClient: indexerClient,
		logger:        logger,
	}

	// Create UCAN server with handlers
	ucanSrv, err := svc.createUCANServer()
	if err != nil {
		return nil, fmt.Errorf("failed to create UCAN server: %w", err)
	}
	svc.ucanServer = ucanSrv

	return svc, nil
}

// createUCANServer creates the UCAN RPC server with registered handlers.
func (s *Service) createUCANServer() (server.ServerView[server.Service], error) {
	// For local development, allow any principal to invoke any capability.
	// This bypasses strict authorization checks that require proper delegation chains.
	permissiveCanIssue := func(cap ucan.Capability[any], issuer did.DID) bool {
		s.logger.Debug("permissive auth check",
			zap.String("capability", cap.Can()),
			zap.String("resource", cap.With()),
			zap.String("issuer", issuer.String()),
		)
		return true // Allow all invocations
	}

	options := []server.Option{
		server.WithCanIssue(permissiveCanIssue),
		handlers.WithAccessAuthorizeMethod(s),
		handlers.WithAccessClaimMethod(s),
		handlers.WithAccessDelegateMethod(s),
		handlers.WithFilecoinOfferMethod(s),
		handlers.WithProviderAddMethod(s),
		handlers.WithSpaceBlobAddMethod(s),
		handlers.WithSpaceBlobReplicateMethod(s),
		handlers.WithSpaceIndexAddMethod(s),
		handlers.WithUploadAddMethod(s),
		handlers.WithUCANConcludeMethod(s),
	}

	return server.NewServer(s.identity.Signer, options...)
}

// HandleUCANRequest handles incoming UCAN RPC requests.
func (s *Service) HandleUCANRequest(c echo.Context) error {
	r := c.Request()

	res, err := s.ucanServer.Request(r.Context(), ucanhttp.NewRequest(r.Body, r.Header))
	if err != nil {
		s.logger.Error("UCAN request error", zap.Error(err))
		return fmt.Errorf("handling UCAN request: %w", err)
	}

	// Copy response headers
	for key, vals := range res.Headers() {
		for _, v := range vals {
			c.Response().Header().Add(key, v)
		}
	}

	return c.Stream(res.Status(), "", res.Body())
}

// receiptWithExtraBlocks wraps a receipt and includes additional blocks in its Blocks() iterator.
type receiptWithExtraBlocks struct {
	receipt.AnyReceipt
	extraBlocks []block.Block
}

func (r *receiptWithExtraBlocks) Blocks() iter.Seq2[block.Block, error] {
	return func(yield func(block.Block, error) bool) {
		// First yield all blocks from the wrapped receipt
		for blk, err := range r.AnyReceipt.Blocks() {
			if !yield(blk, err) {
				return
			}
		}
		// Then yield extra blocks
		for _, blk := range r.extraBlocks {
			if !yield(blk, nil) {
				return
			}
		}
	}
}

// Implement ipld.View interface
func (r *receiptWithExtraBlocks) Root() ipld.Block {
	return r.AnyReceipt.Root()
}

// Implement receipt.Receipt interface methods that delegate to wrapped receipt
func (r *receiptWithExtraBlocks) Ran() ran.Ran {
	return r.AnyReceipt.Ran()
}

func (r *receiptWithExtraBlocks) Out() result.Result[ipld.Node, ipld.Node] {
	return r.AnyReceipt.Out()
}

func (r *receiptWithExtraBlocks) Fx() fx.Effects {
	return r.AnyReceipt.Fx()
}

func (r *receiptWithExtraBlocks) Meta() map[string]any {
	return r.AnyReceipt.Meta()
}

func (r *receiptWithExtraBlocks) Issuer() ucan.Principal {
	return r.AnyReceipt.Issuer()
}

func (r *receiptWithExtraBlocks) Proofs() delegation.Proofs {
	return r.AnyReceipt.Proofs()
}

func (r *receiptWithExtraBlocks) Signature() signature.SignatureView {
	return r.AnyReceipt.Signature()
}

func (r *receiptWithExtraBlocks) VerifySignature(verifier signature.Verifier) (bool, error) {
	return r.AnyReceipt.VerifySignature(verifier)
}

func (r *receiptWithExtraBlocks) Archive() io.Reader {
	return r.AnyReceipt.Archive()
}

func (r *receiptWithExtraBlocks) Export() iter.Seq2[block.Block, error] {
	return r.Blocks() // Same as Blocks for our purposes
}

func (r *receiptWithExtraBlocks) Clone() (receipt.Receipt[ipld.Node, ipld.Node], error) {
	return r.AnyReceipt.Clone()
}

func (r *receiptWithExtraBlocks) AttachInvocation(inv invocation.Invocation) error {
	return r.AnyReceipt.AttachInvocation(inv)
}

// HandleReceiptRequest handles receipt retrieval requests.
func (s *Service) HandleReceiptRequest(c echo.Context) error {
	cidStr := c.Param("cid")
	if cidStr == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "missing cid parameter",
		})
	}

	s.logger.Debug("receipt request", zap.String("cid", cidStr))

	storedRcpt, err := s.state.GetReceipt(c.Request().Context(), cidStr)
	if err != nil {
		s.logger.Error("failed to get receipt", zap.String("cid", cidStr), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to get receipt",
		})
	}
	if storedRcpt == nil {
		s.logger.Debug("receipt not found", zap.String("cid", cidStr))
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "receipt not found",
		})
	}

	s.logger.Debug("receipt found, encoding as CAR",
		zap.String("cid", cidStr),
		zap.Int("extra_blocks", len(storedRcpt.ExtraBlocks)))

	// Wrap the receipt with extra blocks if present
	var rcpt receipt.AnyReceipt = storedRcpt.Receipt
	if len(storedRcpt.ExtraBlocks) > 0 {
		rcpt = &receiptWithExtraBlocks{
			AnyReceipt:  storedRcpt.Receipt,
			extraBlocks: storedRcpt.ExtraBlocks,
		}
	}

	// Build an agent message containing the receipt
	msg, err := message.Build(nil, []receipt.AnyReceipt{rcpt})
	if err != nil {
		s.logger.Error("failed to build message", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to build message",
		})
	}

	// Encode the message as CAR using the outbound codec
	codec := car.NewOutboundCodec()
	req, err := codec.Encode(msg)
	if err != nil {
		s.logger.Error("failed to encode message as CAR", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to encode message",
		})
	}

	// Read the body into a buffer
	body, err := io.ReadAll(req.Body())
	if err != nil {
		s.logger.Error("failed to read CAR body", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to read body",
		})
	}

	// Copy headers
	for key, vals := range req.Headers() {
		for _, v := range vals {
			c.Response().Header().Add(key, v)
		}
	}

	return c.Stream(http.StatusOK, "application/car", bytes.NewReader(body))
}

// Identity returns the service identity.
func (s *Service) Identity() *identity.Identity {
	return s.identity
}

// State returns the state store.
func (s *Service) State() state.StateStore {
	return s.state
}

// Config returns the service configuration.
func (s *Service) Config() *config.Config {
	return s.cfg
}

// Logger returns the logger.
func (s *Service) Logger() *zap.Logger {
	return s.logger
}

// ID returns the service's signer (implements handler interfaces).
func (s *Service) ID() principal.Signer {
	return s.identity.Signer
}

// IndexerClient returns the indexer client (may be nil if not configured).
func (s *Service) IndexerClient() *indexerclient.Client {
	return s.indexerClient
}

// PiriClient queries the provider table and creates a piri client on-demand.
// This ensures we always use the most up-to-date provider information.
func (s *Service) PiriClient(ctx context.Context) (*piriclient.Client, error) {
	// Query provider from DynamoDB (delegator-provider-info table)
	provider, err := s.state.GetFirstProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider from state: %w", err)
	}

	if provider == nil || provider.DID == (did.DID{}) || provider.Endpoint == nil {
		s.logger.Debug("no storage provider registered")
		return nil, nil
	}

	// Create a new piri client for this provider
	client, err := piriclient.New(provider.Endpoint, provider.DID, s.identity.Signer, s.state)
	if err != nil {
		return nil, fmt.Errorf("failed to create piri client: %w", err)
	}

	s.logger.Debug("created piri client from provider info",
		zap.String("did", provider.DID.String()),
		zap.String("endpoint", provider.Endpoint.String()),
	)

	return client, nil
}
