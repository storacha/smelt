package providerstore

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/storacha/go-ucanto/core/delegation"
)

// ProviderInfo contains information about a registered storage provider.
type ProviderInfo struct {
	Provider   string
	Endpoint   string
	Proof      string
	Delegation delegation.Delegation
}

// Store provides access to provider information stored in DynamoDB.
type Store struct {
	db        *dynamodb.Client
	tableName string
}

// New creates a new provider store that reads from DynamoDB.
func New(endpoint, region, tableName string) (*Store, error) {
	ctx := context.Background()

	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
	}

	if endpoint != "" {
		opts = append(opts, awsconfig.WithBaseEndpoint(endpoint))
		// Use dummy credentials for local DynamoDB
		opts = append(opts, awsconfig.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID:     "dummy",
				SecretAccessKey: "dummy",
			},
		}))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	client := dynamodb.NewFromConfig(cfg)

	return &Store{
		db:        client,
		tableName: tableName,
	}, nil
}

// SelectProvider selects a provider for storing blobs.
// TODO: Implement weighted random selection based on provider weights.
// For now, this just returns the first provider in the table.
func (s *Store) SelectProvider(ctx context.Context) (*ProviderInfo, error) {
	input := &dynamodb.ScanInput{
		TableName: aws.String(s.tableName),
		Limit:     aws.Int32(10), // Limit scan for efficiency
	}

	result, err := s.db.Scan(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("scanning provider table: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, nil
	}

	// TODO: Implement weighted random selection using the weight field.
	// For now, just return the first provider.
	item := result.Items[0]

	info := &ProviderInfo{}
	if v, ok := item["provider"].(*types.AttributeValueMemberS); ok {
		info.Provider = v.Value
	}
	if v, ok := item["endpoint"].(*types.AttributeValueMemberS); ok {
		info.Endpoint = v.Value
	}
	if v, ok := item["proof"].(*types.AttributeValueMemberS); ok {
		info.Proof = v.Value
		// Parse the delegation
		dlg, err := delegation.Parse(v.Value)
		if err != nil {
			return nil, fmt.Errorf("parsing delegation: %w", err)
		}
		info.Delegation = dlg
	}

	return info, nil
}

// GetProviderInfo retrieves provider information by DID.
func (s *Store) GetProviderInfo(ctx context.Context, providerDID string) (*ProviderInfo, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"provider": &types.AttributeValueMemberS{Value: providerDID},
		},
	}

	result, err := s.db.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("getting provider info: %w", err)
	}

	if len(result.Item) == 0 {
		return nil, nil
	}

	info := &ProviderInfo{
		Provider: providerDID,
	}

	if v, ok := result.Item["endpoint"].(*types.AttributeValueMemberS); ok {
		info.Endpoint = v.Value
	}
	if v, ok := result.Item["proof"].(*types.AttributeValueMemberS); ok {
		info.Proof = v.Value
		// Parse the delegation
		dlg, err := delegation.Parse(v.Value)
		if err != nil {
			return nil, fmt.Errorf("parsing delegation: %w", err)
		}
		info.Delegation = dlg
	}

	return info, nil
}
