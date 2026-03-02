#!/bin/bash
#
# MDDB MCP Service Authentication Test
#
# This script tests the MCP service with API key authentication
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

TESTS_PASSED=0
TESTS_FAILED=0

print_header() {
    echo -e "\n${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n"
}

print_test() {
    echo -e "${YELLOW}▶ Test $1: $2${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
    ((TESTS_PASSED++))
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
    ((TESTS_FAILED++))
}

print_info() {
    echo -e "  $1"
}

cleanup() {
    echo -e "\n${YELLOW}Cleaning up...${NC}"
    pkill -f mddb-server 2>/dev/null || true
    pkill -f mddb-mcp 2>/dev/null || true
    rm -f services/mddbd/mddb.db services/mddbd/mddb.db.lock
    echo -e "${GREEN}Cleanup complete${NC}"
}

trap cleanup EXIT

print_header "MDDB MCP Service Authentication Test"

# Check directory
if [ ! -f "services/mddb-mcp/go.mod" ]; then
    print_error "Please run this script from the MDDB root directory"
    exit 1
fi

# Build the server
print_header "Building MDDB Server"
cd services/mddbd
if go build -o mddb-server . > /tmp/mddb-build.log 2>&1; then
    print_success "Server built successfully"
else
    print_error "Server build failed"
    cat /tmp/mddb-build.log
    exit 1
fi

# Start server with auth
print_header "Starting MDDB Server with Authentication"
export MDDB_AUTH_ENABLED=true
export MDDB_AUTH_JWT_SECRET=$(openssl rand -hex 32)
export MDDB_AUTH_ADMIN_USERNAME=admin
export MDDB_AUTH_ADMIN_PASSWORD=changeme

./mddb-server > /tmp/mddb-server.log 2>&1 &
SERVER_PID=$!
print_info "Server PID: $SERVER_PID"

sleep 3

if ! ps -p $SERVER_PID > /dev/null; then
    print_error "Server failed to start"
    cat /tmp/mddb-server.log
    exit 1
fi
print_success "Server is running"

cd ../..

# Build MCP service
print_header "Building MCP Service"
cd services/mddb-mcp
if go build -o mddb-mcp ./cmd/mddb-mcp > /tmp/mcp-build.log 2>&1; then
    print_success "MCP service built successfully"
else
    print_error "MCP build failed"
    cat /tmp/mcp-build.log
    exit 1
fi

cd ../..

# ========================================
# Test 1: Login and get token
# ========================================
print_header "Test 1: Get Admin Token"
print_test "1" "Login as admin to get token"

TOKEN=$(curl -s http://localhost:11023/v1/auth/login \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"changeme"}' | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

if [ -n "$TOKEN" ]; then
    print_success "Token obtained: ${TOKEN:0:50}..."
else
    print_error "Failed to get token"
    exit 1
fi

# ========================================
# Test 2: Create API key for MCP
# ========================================
print_header "Test 2: Create API Key for MCP"
print_test "2" "Generate API key for MCP service"

API_KEY=$(curl -s -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    http://localhost:11023/v1/auth/api-key \
    -d '{"description":"MCP service key"}' | grep -o '"key":"[^"]*"' | cut -d'"' -f4)

if [[ "$API_KEY" =~ ^mddb_live_ ]]; then
    print_success "API key created: ${API_KEY:0:30}..."
    print_info "Full key: $API_KEY"
else
    print_error "Failed to create API key"
    exit 1
fi

# ========================================
# Test 3: Test MCP without API key (should fail)
# ========================================
print_header "Test 3: MCP without Authentication"
print_test "3" "Try MCP tools without API key (should fail)"

cd services/mddb-mcp

# Create temporary config without API key
cat > /tmp/mcp-test-noauth.yaml << EOF
mddb:
  grpcAddress: "localhost:11024"
  restBaseURL: "http://localhost:11023/v1"
  transportMode: "rest-only"
  timeout: 30s
server:
  httpPort: 8080
  enableHTTP: false
  enableStdio: true
EOF

# Try to use MCP without auth (should fail)
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"mddb_stats","arguments":{}}}' | \
  timeout 5 ./mddb-mcp --config /tmp/mcp-test-noauth.yaml 2>&1 | grep -q "error" && \
  print_success "MCP correctly fails without authentication" || \
  print_error "MCP should have failed without API key"

# ========================================
# Test 4: Test MCP with API key (should succeed)
# ========================================
print_header "Test 4: MCP with API Key Authentication"
print_test "4" "Use MCP tools with API key (should succeed)"

# Create config with API key
cat > /tmp/mcp-test-auth.yaml << EOF
mddb:
  grpcAddress: "localhost:11024"
  restBaseURL: "http://localhost:11023/v1"
  transportMode: "rest-only"
  timeout: 30s
  apiKey: "$API_KEY"
server:
  httpPort: 8080
  enableHTTP: false
  enableStdio: true
EOF

# Try to use MCP with auth (should succeed)
RESULT=$(echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"mddb_stats","arguments":{}}}' | \
  timeout 5 ./mddb-mcp --config /tmp/mcp-test-auth.yaml 2>&1)

if echo "$RESULT" | grep -q "databasePath"; then
    print_success "MCP successfully authenticated and retrieved stats"
    print_info "$(echo "$RESULT" | grep -o '"databasePath":"[^"]*"')"
else
    print_error "MCP authentication failed or didn't get expected response"
    print_info "Response: $RESULT"
fi

# ========================================
# Test 5: Test MCP tool - add document
# ========================================
print_header "Test 5: MCP Tool - Add Document"
print_test "5" "Add document via MCP (requires write permission)"

RESULT=$(echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"mddb_add","arguments":{"collection":"test","key":"doc1","lang":"en","contentMd":"# Test Document","meta":{"author":["MCP Test"]}}}}' | \
  timeout 5 ./mddb-mcp --config /tmp/mcp-test-auth.yaml 2>&1)

if echo "$RESULT" | grep -q "doc1"; then
    print_success "Document added successfully via MCP"
    print_info "$(echo "$RESULT" | head -c 150)..."
else
    print_error "Failed to add document via MCP"
    print_info "Response: $(echo "$RESULT" | head -c 200)"
fi

# ========================================
# Test 6: Test MCP tool - search
# ========================================
print_header "Test 6: MCP Tool - Search Documents"
print_test "6" "Search documents via MCP"

RESULT=$(echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"mddb_search","arguments":{"collection":"test","limit":10}}}' | \
  timeout 5 ./mddb-mcp --config /tmp/mcp-test-auth.yaml 2>&1)

if echo "$RESULT" | grep -q "doc1"; then
    print_success "Search via MCP works correctly"
else
    print_error "Search via MCP failed"
fi

# ========================================
# Test 7: Test with environment variable
# ========================================
print_header "Test 7: MCP with Environment Variable"
print_test "7" "Use MDDB_API_KEY environment variable"

# Config without apiKey field
cat > /tmp/mcp-test-env.yaml << EOF
mddb:
  grpcAddress: "localhost:11024"
  restBaseURL: "http://localhost:11023/v1"
  transportMode: "rest-only"
  timeout: 30s
server:
  httpPort: 8080
  enableHTTP: false
  enableStdio: true
EOF

# Set API key via environment
export MDDB_API_KEY=$API_KEY

RESULT=$(echo '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"mddb_stats","arguments":{}}}' | \
  timeout 5 ./mddb-mcp --config /tmp/mcp-test-env.yaml 2>&1)

if echo "$RESULT" | grep -q "databasePath"; then
    print_success "MCP works with MDDB_API_KEY environment variable"
else
    print_error "MCP failed with environment variable"
fi

cd ../..

# Cleanup temp files
rm -f /tmp/mcp-test-*.yaml

# ========================================
# Summary
# ========================================
print_header "Test Summary"

TOTAL_TESTS=$((TESTS_PASSED + TESTS_FAILED))

echo -e "${BLUE}Total Tests:${NC} $TOTAL_TESTS"
echo -e "${GREEN}Passed:${NC} $TESTS_PASSED"
echo -e "${RED}Failed:${NC} $TESTS_FAILED"

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "\n${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}   ✓ ALL MCP TESTS PASSED!${NC}"
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n"
    exit 0
else
    echo -e "\n${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${RED}   ✗ SOME MCP TESTS FAILED${NC}"
    echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n"
    exit 1
fi
