package dynamo

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/ipld/block"
	"github.com/storacha/go-ucanto/core/receipt"
	"github.com/storacha/go-ucanto/did"

	"github.com/storacha/smelt/mock-upload-service/pkg/state"
)

// Config holds the DynamoDB store configuration.
type Config struct {
	Endpoint           string
	Region             string
	ProviderInfoTable  string
	AllocationsTable   string
	ReceiptsTable      string
	AuthRequestsTable  string
	ProvisioningsTable string
	UploadsTable       string
}

// Store provides DynamoDB-backed state persistence.
type Store struct {
	db     *dynamodb.Client
	config Config
}

// Ensure Store implements state.StateStore interface
var _ state.StateStore = (*Store)(nil)

// New creates a new DynamoDB-backed state store.
func New(ctx context.Context, cfg Config) (*Store, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}

	if cfg.Endpoint != "" {
		opts = append(opts, awsconfig.WithBaseEndpoint(cfg.Endpoint))
		opts = append(opts, awsconfig.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID:     "dummy",
				SecretAccessKey: "dummy",
			},
		}))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	client := dynamodb.NewFromConfig(awsCfg)

	store := &Store{
		db:     client,
		config: cfg,
	}

	// Create tables if in dev mode (endpoint specified)
	if cfg.Endpoint != "" {
		if err := store.ensureTables(ctx); err != nil {
			return nil, fmt.Errorf("ensuring tables: %w", err)
		}
	}

	return store, nil
}

// ensureTables creates all required tables if they don't exist.
func (s *Store) ensureTables(ctx context.Context) error {
	tables := []struct {
		name       string
		keySchema  []types.KeySchemaElement
		attributes []types.AttributeDefinition
		gsi        []types.GlobalSecondaryIndex
	}{
		{
			name: s.config.AllocationsTable,
			keySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("digest"), KeyType: types.KeyTypeHash},
			},
			attributes: []types.AttributeDefinition{
				{AttributeName: aws.String("digest"), AttributeType: types.ScalarAttributeTypeS},
			},
		},
		{
			name: s.config.ReceiptsTable,
			keySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("task_cid"), KeyType: types.KeyTypeHash},
			},
			attributes: []types.AttributeDefinition{
				{AttributeName: aws.String("task_cid"), AttributeType: types.ScalarAttributeTypeS},
			},
		},
		{
			name: s.config.AuthRequestsTable,
			keySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("link_cid"), KeyType: types.KeyTypeHash},
			},
			attributes: []types.AttributeDefinition{
				{AttributeName: aws.String("link_cid"), AttributeType: types.ScalarAttributeTypeS},
				{AttributeName: aws.String("agent_did"), AttributeType: types.ScalarAttributeTypeS},
			},
			gsi: []types.GlobalSecondaryIndex{
				{
					IndexName: aws.String("agent_did_index"),
					KeySchema: []types.KeySchemaElement{
						{AttributeName: aws.String("agent_did"), KeyType: types.KeyTypeHash},
					},
					Projection: &types.Projection{
						ProjectionType: types.ProjectionTypeAll,
					},
				},
			},
		},
		{
			name: s.config.ProvisioningsTable,
			keySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("space_did"), KeyType: types.KeyTypeHash},
			},
			attributes: []types.AttributeDefinition{
				{AttributeName: aws.String("space_did"), AttributeType: types.ScalarAttributeTypeS},
			},
		},
		{
			name: s.config.UploadsTable,
			keySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("space_did"), KeyType: types.KeyTypeHash},
				{AttributeName: aws.String("root_cid"), KeyType: types.KeyTypeRange},
			},
			attributes: []types.AttributeDefinition{
				{AttributeName: aws.String("space_did"), AttributeType: types.ScalarAttributeTypeS},
				{AttributeName: aws.String("root_cid"), AttributeType: types.ScalarAttributeTypeS},
			},
		},
	}

	for _, table := range tables {
		_, err := s.db.DescribeTable(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(table.name),
		})
		if err == nil {
			continue // table exists
		}

		input := &dynamodb.CreateTableInput{
			TableName:            aws.String(table.name),
			KeySchema:            table.keySchema,
			AttributeDefinitions: table.attributes,
			BillingMode:          types.BillingModePayPerRequest,
		}
		if len(table.gsi) > 0 {
			input.GlobalSecondaryIndexes = table.gsi
		}

		_, err = s.db.CreateTable(ctx, input)
		if err != nil {
			return fmt.Errorf("creating table %s: %w", table.name, err)
		}
	}

	return nil
}

// PutAllocation stores an allocation.
func (s *Store) PutAllocation(ctx context.Context, key string, alloc *state.Allocation) error {
	item := map[string]types.AttributeValue{
		"digest":     &types.AttributeValueMemberS{Value: key},
		"space":      &types.AttributeValueMemberS{Value: alloc.Space.String()},
		"digest_raw": &types.AttributeValueMemberS{Value: hex.EncodeToString(alloc.Digest)},
		"size":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", alloc.Size)},
		"expires_at": &types.AttributeValueMemberS{Value: alloc.ExpiresAt.Format(time.RFC3339)},
		"piri_node":  &types.AttributeValueMemberS{Value: alloc.PiriNode},
	}

	if alloc.Cause != nil {
		item["cause"] = &types.AttributeValueMemberS{Value: alloc.Cause.String()}
	}
	if alloc.UploadURL != nil {
		item["upload_url"] = &types.AttributeValueMemberS{Value: alloc.UploadURL.String()}
	}
	if alloc.AcceptInvLink != nil {
		item["accept_inv_link"] = &types.AttributeValueMemberS{Value: alloc.AcceptInvLink.String()}
	}

	_, err := s.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.config.AllocationsTable),
		Item:      item,
	})
	return err
}

// GetAllocation retrieves an allocation by key.
func (s *Store) GetAllocation(ctx context.Context, key string) (*state.Allocation, error) {
	result, err := s.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.config.AllocationsTable),
		Key: map[string]types.AttributeValue{
			"digest": &types.AttributeValueMemberS{Value: key},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("getting allocation: %w", err)
	}

	if len(result.Item) == 0 {
		return nil, nil
	}

	return parseAllocation(result.Item)
}

// DeleteAllocation removes an allocation.
func (s *Store) DeleteAllocation(ctx context.Context, key string) error {
	_, err := s.db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.config.AllocationsTable),
		Key: map[string]types.AttributeValue{
			"digest": &types.AttributeValueMemberS{Value: key},
		},
	})
	return err
}

func parseAllocation(item map[string]types.AttributeValue) (*state.Allocation, error) {
	alloc := &state.Allocation{}

	if v, ok := item["space"].(*types.AttributeValueMemberS); ok {
		d, err := did.Parse(v.Value)
		if err != nil {
			return nil, fmt.Errorf("parsing space DID: %w", err)
		}
		alloc.Space = d
	}

	if v, ok := item["digest_raw"].(*types.AttributeValueMemberS); ok {
		b, err := hex.DecodeString(v.Value)
		if err != nil {
			return nil, fmt.Errorf("decoding digest: %w", err)
		}
		alloc.Digest = multihash.Multihash(b)
	}

	if v, ok := item["size"].(*types.AttributeValueMemberN); ok {
		var size uint64
		fmt.Sscanf(v.Value, "%d", &size)
		alloc.Size = size
	}

	if v, ok := item["expires_at"].(*types.AttributeValueMemberS); ok {
		t, _ := time.Parse(time.RFC3339, v.Value)
		alloc.ExpiresAt = t
	}

	if v, ok := item["piri_node"].(*types.AttributeValueMemberS); ok {
		alloc.PiriNode = v.Value
	}

	if v, ok := item["upload_url"].(*types.AttributeValueMemberS); ok {
		u, _ := url.Parse(v.Value)
		alloc.UploadURL = u
	}

	if v, ok := item["cause"].(*types.AttributeValueMemberS); ok {
		c, err := cid.Parse(v.Value)
		if err == nil {
			alloc.Cause = cidlink.Link{Cid: c}
		}
	}

	if v, ok := item["accept_inv_link"].(*types.AttributeValueMemberS); ok {
		c, err := cid.Parse(v.Value)
		if err == nil {
			alloc.AcceptInvLink = cidlink.Link{Cid: c}
		}
	}

	return alloc, nil
}

// PutReceipt stores a receipt.
func (s *Store) PutReceipt(ctx context.Context, taskCID string, rcpt *state.StoredReceipt) error {
	item := map[string]types.AttributeValue{
		"task_cid": &types.AttributeValueMemberS{Value: taskCID},
		"added_at": &types.AttributeValueMemberS{Value: rcpt.AddedAt.Format(time.RFC3339)},
	}

	if rcpt.Task != nil {
		item["task"] = &types.AttributeValueMemberS{Value: rcpt.Task.String()}
	}

	// Serialize receipt as CAR
	if rcpt.Receipt != nil {
		carBytes, err := serializeReceiptToCAR(rcpt.Receipt)
		if err != nil {
			return fmt.Errorf("serializing receipt: %w", err)
		}
		item["receipt_car"] = &types.AttributeValueMemberS{Value: base64.StdEncoding.EncodeToString(carBytes)}
	}

	// Serialize extra blocks as JSON array of base64 encoded blocks
	if len(rcpt.ExtraBlocks) > 0 {
		blocksData := make([]map[string]string, len(rcpt.ExtraBlocks))
		for i, blk := range rcpt.ExtraBlocks {
			blocksData[i] = map[string]string{
				"cid":  blk.Link().String(),
				"data": base64.StdEncoding.EncodeToString(blk.Bytes()),
			}
		}
		blocksJSON, _ := json.Marshal(blocksData)
		item["extra_blocks"] = &types.AttributeValueMemberS{Value: string(blocksJSON)}
	}

	_, err := s.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.config.ReceiptsTable),
		Item:      item,
	})
	return err
}

// GetReceipt retrieves a receipt by task CID.
func (s *Store) GetReceipt(ctx context.Context, taskCID string) (*state.StoredReceipt, error) {
	result, err := s.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.config.ReceiptsTable),
		Key: map[string]types.AttributeValue{
			"task_cid": &types.AttributeValueMemberS{Value: taskCID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("getting receipt: %w", err)
	}

	if len(result.Item) == 0 {
		return nil, nil
	}

	return parseStoredReceipt(result.Item)
}

func parseStoredReceipt(item map[string]types.AttributeValue) (*state.StoredReceipt, error) {
	rcpt := &state.StoredReceipt{}

	if v, ok := item["task"].(*types.AttributeValueMemberS); ok {
		c, err := cid.Parse(v.Value)
		if err == nil {
			rcpt.Task = cidlink.Link{Cid: c}
		}
	}

	if v, ok := item["added_at"].(*types.AttributeValueMemberS); ok {
		t, _ := time.Parse(time.RFC3339, v.Value)
		rcpt.AddedAt = t
	}

	if v, ok := item["receipt_car"].(*types.AttributeValueMemberS); ok {
		carBytes, err := base64.StdEncoding.DecodeString(v.Value)
		if err != nil {
			return nil, fmt.Errorf("decoding receipt CAR: %w", err)
		}
		anyRcpt, err := deserializeReceiptFromCAR(carBytes)
		if err != nil {
			return nil, fmt.Errorf("deserializing receipt: %w", err)
		}
		rcpt.Receipt = anyRcpt
	}

	if v, ok := item["extra_blocks"].(*types.AttributeValueMemberS); ok {
		var blocksData []map[string]string
		if err := json.Unmarshal([]byte(v.Value), &blocksData); err != nil {
			return nil, fmt.Errorf("unmarshaling extra blocks: %w", err)
		}
		rcpt.ExtraBlocks = make([]block.Block, 0, len(blocksData))
		for _, bd := range blocksData {
			c, err := cid.Parse(bd["cid"])
			if err != nil {
				continue
			}
			data, err := base64.StdEncoding.DecodeString(bd["data"])
			if err != nil {
				continue
			}
			blk := block.NewBlock(cidlink.Link{Cid: c}, data)
			rcpt.ExtraBlocks = append(rcpt.ExtraBlocks, blk)
		}
	}

	return rcpt, nil
}

// serializeReceiptToCAR converts a receipt to CAR bytes using the Archive() method.
func serializeReceiptToCAR(rcpt receipt.AnyReceipt) ([]byte, error) {
	archiveReader := rcpt.Archive()
	return io.ReadAll(archiveReader)
}

// deserializeReceiptFromCAR reads a receipt from CAR bytes using receipt.Extract().
func deserializeReceiptFromCAR(carBytes []byte) (receipt.AnyReceipt, error) {
	return receipt.Extract(carBytes)
}

// PutAuthRequest stores an authorization request.
func (s *Store) PutAuthRequest(ctx context.Context, linkCID string, req *state.AuthRequest) error {
	item := map[string]types.AttributeValue{
		"link_cid":    &types.AttributeValueMemberS{Value: linkCID},
		"agent_did":   &types.AttributeValueMemberS{Value: req.AgentDID},
		"account_did": &types.AttributeValueMemberS{Value: req.AccountDID},
		"expiration":  &types.AttributeValueMemberS{Value: req.Expiration.Format(time.RFC3339)},
		"claimed":     &types.AttributeValueMemberBOOL{Value: req.Claimed},
	}

	if req.RequestLink != "" {
		item["request_link"] = &types.AttributeValueMemberS{Value: req.RequestLink}
	}

	_, err := s.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.config.AuthRequestsTable),
		Item:      item,
	})
	return err
}

// GetAuthRequest retrieves an authorization request by link CID.
func (s *Store) GetAuthRequest(ctx context.Context, linkCID string) (*state.AuthRequest, error) {
	result, err := s.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.config.AuthRequestsTable),
		Key: map[string]types.AttributeValue{
			"link_cid": &types.AttributeValueMemberS{Value: linkCID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("getting auth request: %w", err)
	}

	if len(result.Item) == 0 {
		return nil, nil
	}

	return parseAuthRequest(result.Item), nil
}

// GetAuthRequestsByAgent retrieves all pending auth requests for an agent using GSI.
func (s *Store) GetAuthRequestsByAgent(ctx context.Context, agentDID string) ([]*state.AuthRequest, error) {
	result, err := s.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.config.AuthRequestsTable),
		IndexName:              aws.String("agent_did_index"),
		KeyConditionExpression: aws.String("agent_did = :agent_did"),
		FilterExpression:       aws.String("claimed = :claimed"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":agent_did": &types.AttributeValueMemberS{Value: agentDID},
			":claimed":   &types.AttributeValueMemberBOOL{Value: false},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("querying auth requests: %w", err)
	}

	requests := make([]*state.AuthRequest, 0, len(result.Items))
	for _, item := range result.Items {
		requests = append(requests, parseAuthRequest(item))
	}
	return requests, nil
}

// MarkAuthRequestClaimed marks an auth request as claimed.
func (s *Store) MarkAuthRequestClaimed(ctx context.Context, linkCID string) error {
	_, err := s.db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.config.AuthRequestsTable),
		Key: map[string]types.AttributeValue{
			"link_cid": &types.AttributeValueMemberS{Value: linkCID},
		},
		UpdateExpression: aws.String("SET claimed = :claimed"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":claimed": &types.AttributeValueMemberBOOL{Value: true},
		},
	})
	return err
}

func parseAuthRequest(item map[string]types.AttributeValue) *state.AuthRequest {
	req := &state.AuthRequest{}

	if v, ok := item["agent_did"].(*types.AttributeValueMemberS); ok {
		req.AgentDID = v.Value
	}
	if v, ok := item["account_did"].(*types.AttributeValueMemberS); ok {
		req.AccountDID = v.Value
	}
	if v, ok := item["request_link"].(*types.AttributeValueMemberS); ok {
		req.RequestLink = v.Value
	}
	if v, ok := item["expiration"].(*types.AttributeValueMemberS); ok {
		t, _ := time.Parse(time.RFC3339, v.Value)
		req.Expiration = t
	}
	if v, ok := item["claimed"].(*types.AttributeValueMemberBOOL); ok {
		req.Claimed = v.Value
	}

	return req
}

// PutProvisioning stores a space provisioning record.
func (s *Store) PutProvisioning(ctx context.Context, spaceDID string, prov *state.Provisioning) error {
	_, err := s.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.config.ProvisioningsTable),
		Item: map[string]types.AttributeValue{
			"space_did": &types.AttributeValueMemberS{Value: spaceDID},
			"account":   &types.AttributeValueMemberS{Value: prov.Account},
			"provider":  &types.AttributeValueMemberS{Value: prov.Provider},
			"space":     &types.AttributeValueMemberS{Value: prov.Space},
		},
	})
	return err
}

// GetProvisioning retrieves a provisioning by space DID.
func (s *Store) GetProvisioning(ctx context.Context, spaceDID string) (*state.Provisioning, error) {
	result, err := s.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.config.ProvisioningsTable),
		Key: map[string]types.AttributeValue{
			"space_did": &types.AttributeValueMemberS{Value: spaceDID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("getting provisioning: %w", err)
	}

	if len(result.Item) == 0 {
		return nil, nil
	}

	prov := &state.Provisioning{}
	if v, ok := result.Item["account"].(*types.AttributeValueMemberS); ok {
		prov.Account = v.Value
	}
	if v, ok := result.Item["provider"].(*types.AttributeValueMemberS); ok {
		prov.Provider = v.Value
	}
	if v, ok := result.Item["space"].(*types.AttributeValueMemberS); ok {
		prov.Space = v.Value
	}

	return prov, nil
}

// PutUpload stores an upload record.
func (s *Store) PutUpload(ctx context.Context, spaceDID string, upload *state.Upload) error {
	rootCID := ""
	if upload.Root != nil {
		rootCID = upload.Root.String()
	}

	shardStrings := make([]string, len(upload.Shards))
	for i, shard := range upload.Shards {
		if shard != nil {
			shardStrings[i] = shard.String()
		}
	}
	shardsJSON, _ := json.Marshal(shardStrings)

	_, err := s.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.config.UploadsTable),
		Item: map[string]types.AttributeValue{
			"space_did": &types.AttributeValueMemberS{Value: spaceDID},
			"root_cid":  &types.AttributeValueMemberS{Value: rootCID},
			"space":     &types.AttributeValueMemberS{Value: upload.Space.String()},
			"shards":    &types.AttributeValueMemberS{Value: string(shardsJSON)},
			"added_at":  &types.AttributeValueMemberS{Value: upload.AddedAt.Format(time.RFC3339)},
		},
	})
	return err
}

// GetUploads retrieves all uploads for a space.
func (s *Store) GetUploads(ctx context.Context, spaceDID string) ([]*state.Upload, error) {
	result, err := s.db.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.config.UploadsTable),
		KeyConditionExpression: aws.String("space_did = :space_did"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":space_did": &types.AttributeValueMemberS{Value: spaceDID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("querying uploads: %w", err)
	}

	uploads := make([]*state.Upload, 0, len(result.Items))
	for _, item := range result.Items {
		upload := &state.Upload{}

		if v, ok := item["space"].(*types.AttributeValueMemberS); ok {
			d, _ := did.Parse(v.Value)
			upload.Space = d
		}
		if v, ok := item["root_cid"].(*types.AttributeValueMemberS); ok {
			c, _ := cid.Parse(v.Value)
			upload.Root = cidlink.Link{Cid: c}
		}
		if v, ok := item["shards"].(*types.AttributeValueMemberS); ok {
			var shardStrings []string
			json.Unmarshal([]byte(v.Value), &shardStrings)
			upload.Shards = make([]ipld.Link, len(shardStrings))
			for i, s := range shardStrings {
				c, _ := cid.Parse(s)
				upload.Shards[i] = cidlink.Link{Cid: c}
			}
		}
		if v, ok := item["added_at"].(*types.AttributeValueMemberS); ok {
			t, _ := time.Parse(time.RFC3339, v.Value)
			upload.AddedAt = t
		}

		uploads = append(uploads, upload)
	}

	return uploads, nil
}

// ProviderInfo contains information about a registered storage provider.
type ProviderInfo struct {
	Provider   string
	Endpoint   string
	Proof      string
	Delegation delegation.Delegation
}

// GetProviderInfo retrieves provider information by DID from the delegator table.
func (s *Store) GetProviderInfo(ctx context.Context, providerDID string) (*ProviderInfo, error) {
	log.Printf("[dynamo] GetProviderInfo: looking up provider=%s in table=%s", providerDID, s.config.ProviderInfoTable)

	result, err := s.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.config.ProviderInfoTable),
		Key: map[string]types.AttributeValue{
			"provider": &types.AttributeValueMemberS{Value: providerDID},
		},
	})
	if err != nil {
		log.Printf("[dynamo] GetProviderInfo: error querying DynamoDB: %v", err)
		return nil, fmt.Errorf("getting provider info: %w", err)
	}

	if len(result.Item) == 0 {
		log.Printf("[dynamo] GetProviderInfo: no item found for provider=%s", providerDID)
		return nil, nil
	}

	log.Printf("[dynamo] GetProviderInfo: found item with %d attributes", len(result.Item))

	info := &ProviderInfo{
		Provider: providerDID,
	}

	if v, ok := result.Item["endpoint"].(*types.AttributeValueMemberS); ok {
		info.Endpoint = v.Value
		log.Printf("[dynamo] GetProviderInfo: endpoint=%s", v.Value)
	}
	if v, ok := result.Item["proof"].(*types.AttributeValueMemberS); ok {
		info.Proof = v.Value
		log.Printf("[dynamo] GetProviderInfo: found proof (length=%d)", len(v.Value))
		dlg, err := delegation.Parse(v.Value)
		if err != nil {
			log.Printf("[dynamo] GetProviderInfo: error parsing delegation: %v", err)
			return nil, fmt.Errorf("parsing delegation: %w", err)
		}
		log.Printf("[dynamo] GetProviderInfo: parsed delegation issuer=%s audience=%s", dlg.Issuer().DID().String(), dlg.Audience().DID().String())
		info.Delegation = dlg
	} else {
		log.Printf("[dynamo] GetProviderInfo: no proof field in item")
	}

	return info, nil
}

// GetFirstProvider returns the first provider from the delegator table.
// For simplicity, we query the provider info table.
func (s *Store) GetFirstProvider(ctx context.Context) (*state.Provider, error) {
	// Scan the provider info table to get the first provider
	result, err := s.db.Scan(ctx, &dynamodb.ScanInput{
		TableName: aws.String(s.config.ProviderInfoTable),
		Limit:     aws.Int32(1),
	})
	if err != nil {
		return nil, fmt.Errorf("scanning provider info: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, nil
	}

	item := result.Items[0]
	provider := &state.Provider{
		Weight: 1,
	}

	if v, ok := item["provider"].(*types.AttributeValueMemberS); ok {
		d, err := did.Parse(v.Value)
		if err == nil {
			provider.DID = d
		}
	}
	if v, ok := item["endpoint"].(*types.AttributeValueMemberS); ok {
		u, err := url.Parse(v.Value)
		if err == nil {
			provider.Endpoint = u
		}
	}

	return provider, nil
}

// GetDelegation implements the piriclient.DelegationFetcher interface.
// It fetches the delegation proof for the given provider DID on-demand.
func (s *Store) GetDelegation(ctx context.Context, providerDID string) (delegation.Delegation, error) {
	info, err := s.GetProviderInfo(ctx, providerDID)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, nil
	}
	return info.Delegation, nil
}
