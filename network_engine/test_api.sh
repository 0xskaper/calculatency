#!/bin/bash

# Test script for CalcuLatency API
BASE_URL="http://localhost:8080/api/v1"

echo "Testing CalcuLatency Go Network Engine API..."

# Test health endpoint
echo "1. Testing health endpoint..."
curl -s "$BASE_URL/health" | jq . || echo "Health check failed"

echo -e "\n2. Testing status endpoint..."
curl -s "$BASE_URL/status" | jq . || echo "Status check failed"

echo -e "\n3. Testing measurement endpoint..."
curl -s -X POST "$BASE_URL/measure" \
  -H "Content-Type: application/json" \
  -d '{
    "clientIP": "8.8.8.8",
    "requestID": "test-001",
    "icmpCount": 3,
    "traceMaxTTL": 16
  }' | jq . || echo "Measurement test failed"

echo -e "\n4. Testing analyze endpoint..."
curl -s -X POST "$BASE_URL/analyze" \
  -H "Content-Type: application/json" \
  -d '{
    "clientIP": "8.8.8.8",
    "webSocketRTT": 120
  }' | jq . || echo "Analysis test failed"

echo -e "\nAPI testing complete!"
