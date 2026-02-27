package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/smelt/mock-upload-service/pkg/config"
	"github.com/storacha/smelt/mock-upload-service/pkg/dynamo"
	"github.com/storacha/smelt/mock-upload-service/pkg/identity"
	"github.com/storacha/smelt/mock-upload-service/pkg/indexerclient"
	"github.com/storacha/smelt/mock-upload-service/pkg/service"
	"github.com/storacha/smelt/mock-upload-service/pkg/state"
	"go.uber.org/zap"
)

// Server represents the mock upload HTTP server.
type Server struct {
	cfg      *config.Config
	logger   *zap.Logger
	echo     *echo.Echo
	identity *identity.Identity
	state    state.StateStore
	service  *service.Service
}

// New creates a new Server instance.
func New(cfg *config.Config, logger *zap.Logger) (*Server, error) {
	// Create identity - prefer PEM key file over base64-encoded key
	var id *identity.Identity
	var err error
	if cfg.KeyFile != "" {
		// Use PEM file with optional did:web wrapping
		id, err = identity.NewFromPEMFileWithDID(cfg.KeyFile, cfg.ServiceDID)
		if err != nil {
			return nil, fmt.Errorf("failed to create identity from key file: %w", err)
		}
		if cfg.ServiceDID != "" {
			logger.Info("service identity created from PEM file with did:web wrapping",
				zap.String("did", id.DID()),
				zap.String("key_file", cfg.KeyFile),
				zap.String("service_did", cfg.ServiceDID),
			)
		} else {
			logger.Info("service identity created from PEM file",
				zap.String("did", id.DID()),
				zap.String("key_file", cfg.KeyFile),
			)
		}
	} else {
		id, err = identity.New(cfg.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create identity: %w", err)
		}
		logger.Info("service identity created", zap.String("did", id.DID()))
	}

	// Create DynamoDB-backed state store
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := dynamo.New(ctx, dynamo.Config{
		Endpoint:           cfg.DynamoDBEndpoint,
		Region:             cfg.DynamoDBRegion,
		ProviderInfoTable:  cfg.DynamoDBProviderTable,
		AllocationsTable:   cfg.DynamoDBAllocationsTable,
		ReceiptsTable:      cfg.DynamoDBReceiptsTable,
		AuthRequestsTable:  cfg.DynamoDBAuthRequestsTable,
		ProvisioningsTable: cfg.DynamoDBProvisioningsTable,
		UploadsTable:       cfg.DynamoDBUploadsTable,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create DynamoDB store: %w", err)
	}
	logger.Info("connected to DynamoDB store",
		zap.String("endpoint", cfg.DynamoDBEndpoint),
		zap.String("allocations_table", cfg.DynamoDBAllocationsTable),
		zap.String("receipts_table", cfg.DynamoDBReceiptsTable),
	)

	// Note: Piri client is created on-demand in the service handlers.
	// This allows the upload service to dynamically pick up providers
	// as they register with the delegator.

	// Create indexer client if configured
	var idxClient *indexerclient.Client
	if cfg.IndexerEndpoint != "" {
		indexerURL, err := url.Parse(cfg.IndexerEndpoint)
		if err != nil {
			logger.Warn("failed to parse indexer endpoint",
				zap.String("endpoint", cfg.IndexerEndpoint),
				zap.Error(err),
			)
		} else {
			var indexerDID did.DID
			if cfg.IndexerDID != "" {
				// Use explicitly configured DID
				indexerDID, err = did.Parse(cfg.IndexerDID)
				if err != nil {
					logger.Warn("failed to parse indexer DID",
						zap.String("did", cfg.IndexerDID),
						zap.Error(err),
					)
				}
			} else {
				// Fall back to deriving from hostname (without port)
				indexerDID, err = did.Parse("did:web:" + indexerURL.Hostname())
				if err != nil {
					logger.Warn("failed to create indexer DID",
						zap.String("host", indexerURL.Hostname()),
						zap.Error(err),
					)
				}
			}

			if indexerDID != (did.DID{}) {
				idxClient, err = indexerclient.New(indexerURL, indexerDID, id.Signer)
				if err != nil {
					logger.Warn("failed to create indexer client",
						zap.Error(err),
					)
				} else {
					logger.Info("created indexer client",
						zap.String("endpoint", cfg.IndexerEndpoint),
						zap.String("did", indexerDID.String()),
					)
				}
			}
		}
	}

	// Create service
	svc, err := service.New(cfg, id, store, idxClient, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}

	// Create Echo instance
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// Middleware
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.Logger())

	// Routes
	e.GET("/", infoHandler(id))
	e.GET("/health", healthHandler)
	e.GET("/.well-known/did.json", didDocumentHandler(id))
	e.POST("/", svc.HandleUCANRequest)
	e.GET("/receipt/:cid", svc.HandleReceiptRequest)

	return &Server{
		cfg:      cfg,
		logger:   logger,
		echo:     e,
		identity: id,
		state:    store,
		service:  svc,
	}, nil
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	s.logger.Info("starting mock upload service",
		zap.String("address", addr),
		zap.String("did", s.identity.DID()),
		zap.String("piri_endpoint", s.cfg.PiriEndpoint),
	)

	// Start server in goroutine
	go func() {
		if err := s.echo.Start(addr); err != nil && err != http.ErrServerClosed {
			s.logger.Fatal("server error", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	s.logger.Info("shutting down server")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return s.echo.Shutdown(ctx)
}

// infoHandler returns service information.
func infoHandler(id *identity.Identity) echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"service": "mock-upload-service",
			"did":     id.DID(),
			"version": "0.1.0",
		})
	}
}

// healthHandler returns health status.
func healthHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"status": "healthy",
	})
}

// didDocumentHandler returns the DID document for did:web resolution.
// This allows other services (like piri) to resolve the public key
// for verifying UCANs signed by this service.
func didDocumentHandler(id *identity.Identity) echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, id.DIDDocument())
	}
}
