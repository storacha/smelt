#!/bin/bash
# Register a DID with the delegator allow list (DynamoDB)
#
# Usage: ./register-did.sh <did>
#   <did>: The DID to register (e.g., did:key:z6Mk...)
#
# Environment variables:
#   DYNAMODB_ENDPOINT: DynamoDB endpoint (default: http://dynamodb-local:8000)
#   DYNAMODB_TABLE: Table name (default: delegator-allow-list)

set -e

DID="$1"
if [ -z "$DID" ]; then
    echo "Usage: $0 <did>"
    echo "Error: DID argument required"
    exit 1
fi

DYNAMODB_ENDPOINT="${DYNAMODB_ENDPOINT:-http://dynamodb-local:8000}"
DYNAMODB_TABLE="${DYNAMODB_TABLE:-delegator-allow-list}"

echo "Registering DID with allow list..."
echo "  DID: $DID"
echo "  Endpoint: $DYNAMODB_ENDPOINT"
echo "  Table: $DYNAMODB_TABLE"

DATE=$(date -u +%Y%m%dT%H%M%SZ)

HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
  -H "Content-Type: application/x-amz-json-1.0" \
  -H "X-Amz-Target: DynamoDB_20120810.PutItem" \
  -H "Authorization: AWS4-HMAC-SHA256 Credential=dummy/${DATE:0:8}/us-west-1/dynamodb/aws4_request, SignedHeaders=content-type;host;x-amz-date;x-amz-target, Signature=dummy" \
  -H "X-Amz-Date: $DATE" \
  -d '{
    "TableName": "'"$DYNAMODB_TABLE"'",
    "Item": {
      "did": {"S": "'"$DID"'"},
      "added_by": {"S": "register-did.sh"},
      "added_at": {"S": "'"$(date -u +%Y-%m-%dT%H:%M:%SZ)"'"},
      "notes": {"S": "local dev"}
    }
  }' \
  "$DYNAMODB_ENDPOINT")

if [ "$HTTP_STATUS" = "200" ]; then
    echo "DID registered successfully"
    exit 0
else
    echo "Warning: Registration returned HTTP $HTTP_STATUS"
    exit 1
fi
