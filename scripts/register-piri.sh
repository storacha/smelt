#!/bin/bash
# Adds a DID to the delegator allow list in local DynamoDB
# Usage: ./register-piri.sh <did>
# Example: ./register-piri.sh did:key:z6MkeUPmap4WfZFWgLZuL8gfdFMbDzRQhHZfnxjFmK2NWTm9

set -e

if [ -z "$1" ]; then
    echo "Usage: $0 <did>"
    echo "Example: $0 did:key:z6MkeUPmap4WfZFWgLZuL8gfdFMbDzRQhHZfnxjFmK2NWTm9"
    exit 1
fi

DID="$1"
ENDPOINT_URL="${DYNAMODB_ENDPOINT:-http://localhost:8000}"
TABLE_NAME="${DYNAMODB_TABLE:-delegator-allow-list}"
REGION="${AWS_REGION:-us-west-1}"

echo "Adding DID to allow list..."
echo "  DID: $DID"
echo "  Endpoint: $ENDPOINT_URL"
echo "  Table: $TABLE_NAME"

aws dynamodb put-item \
    --endpoint-url "$ENDPOINT_URL" \
    --table-name "$TABLE_NAME" \
    --item "{\"did\": {\"S\": \"$DID\"}, \"added_by\": {\"S\": \"register-piri.sh\"}, \"added_at\": {\"S\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}, \"notes\": {\"S\": \"local dev testing\"}}" \
    --region "$REGION"

echo "Done! DID added to allow list."
