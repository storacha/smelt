package config

import (
	"os"
	"strconv"
)

// Config holds the mock upload service configuration.
type Config struct {
	// Host is the address to bind the HTTP server to.
	Host string

	// Port is the port to listen on.
	Port int

	// PiriEndpoint is the URL of the Piri node to route uploads to.
	PiriEndpoint string

	// PiriDID is the DID of the Piri node (optional, can be discovered).
	PiriDID string

	// IndexerEndpoint is the URL of the indexing service.
	IndexerEndpoint string

	// IndexerDID is the DID of the indexer service (optional, can be derived from endpoint).
	IndexerDID string

	// PrivateKey is the base64-encoded ed25519 private key for service identity.
	// If empty, a key will be generated at startup.
	PrivateKey string

	// KeyFile is the path to an Ed25519 PEM key file for service identity.
	// Takes precedence over PrivateKey if set.
	KeyFile string

	// ServiceDID is the did:web identity for this service (e.g., did:web:upload).
	// When set with KeyFile, the service will wrap its did:key signer with this did:web
	// identity, allowing it to accept UCANs addressed to the did:web.
	ServiceDID string

	// LogLevel controls logging verbosity (debug, info, warn, error).
	LogLevel string

	// DynamoDBEndpoint is the endpoint for DynamoDB (for local development).
	DynamoDBEndpoint string

	// DynamoDBRegion is the AWS region for DynamoDB.
	DynamoDBRegion string

	// DynamoDBProviderTable is the table name for provider info.
	DynamoDBProviderTable string

	// DynamoDBAllocationsTable is the table name for blob allocations.
	DynamoDBAllocationsTable string

	// DynamoDBReceiptsTable is the table name for UCAN receipts.
	DynamoDBReceiptsTable string

	// DynamoDBAuthRequestsTable is the table name for auth requests.
	DynamoDBAuthRequestsTable string

	// DynamoDBProvisioningsTable is the table name for space provisionings.
	DynamoDBProvisioningsTable string

	// DynamoDBUploadsTable is the table name for uploads.
	DynamoDBUploadsTable string
}

// Load loads configuration from environment variables.
func Load() (*Config, error) {
	port := 8080
	if p := os.Getenv("PORT"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil {
			port = parsed
		}
	}

	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	return &Config{
		Host:                  host,
		Port:                  port,
		PiriEndpoint:          getEnvOrDefault("PIRI_ENDPOINT", "http://piri:3000"),
		PiriDID:               os.Getenv("PIRI_DID"),
		IndexerEndpoint:       getEnvOrDefault("INDEXER_ENDPOINT", "http://indexer:9000"),
		IndexerDID:            os.Getenv("INDEXER_DID"),
		PrivateKey:            os.Getenv("PRIVATE_KEY"),
		KeyFile:               os.Getenv("KEY_FILE"),
		ServiceDID:            os.Getenv("SERVICE_DID"),
		LogLevel:              getEnvOrDefault("LOG_LEVEL", "info"),
		DynamoDBEndpoint:           getEnvOrDefault("DYNAMODB_ENDPOINT", "http://dynamodb-local:8000"),
		DynamoDBRegion:             getEnvOrDefault("DYNAMODB_REGION", "us-west-1"),
		DynamoDBProviderTable:      getEnvOrDefault("DYNAMODB_PROVIDER_TABLE", "delegator-provider-info"),
		DynamoDBAllocationsTable:   getEnvOrDefault("DYNAMODB_ALLOCATIONS_TABLE", "upload-allocations"),
		DynamoDBReceiptsTable:      getEnvOrDefault("DYNAMODB_RECEIPTS_TABLE", "upload-receipts"),
		DynamoDBAuthRequestsTable:  getEnvOrDefault("DYNAMODB_AUTH_REQUESTS_TABLE", "upload-auth-requests"),
		DynamoDBProvisioningsTable: getEnvOrDefault("DYNAMODB_PROVISIONINGS_TABLE", "upload-provisionings"),
		DynamoDBUploadsTable:       getEnvOrDefault("DYNAMODB_UPLOADS_TABLE", "upload-uploads"),
	}, nil
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
