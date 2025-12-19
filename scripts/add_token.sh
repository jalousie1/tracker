#!/bin/bash

# Script para adicionar token via API
# Uso: ./add_token.sh <TOKEN> <OWNER_USER_ID> [ADMIN_KEY] [API_URL]

TOKEN="$1"
OWNER_USER_ID="$2"
ADMIN_KEY="${3:-${ADMIN_SECRET_KEY}}"
API_URL="${4:-http://localhost:8080}"

if [ -z "$TOKEN" ] || [ -z "$OWNER_USER_ID" ]; then
    echo "Uso: $0 <TOKEN> <OWNER_USER_ID> [ADMIN_KEY] [API_URL]"
    exit 1
fi

if [ -z "$ADMIN_KEY" ]; then
    echo "Erro: ADMIN_SECRET_KEY não definido e não fornecido como argumento"
    exit 1
fi

curl -X POST "$API_URL/api/v1/admin/tokens" \
  -H "Content-Type: application/json" \
  -H "X-Admin-Key: $ADMIN_KEY" \
  -d "{\"token\": \"$TOKEN\", \"owner_user_id\": \"$OWNER_USER_ID\"}"

echo ""

