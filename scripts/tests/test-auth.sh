#!/bin/bash
#
# MDDB Authentication & Authorization Test Suite
#
# This script tests the complete JWT authentication and RBAC implementation
# across all MDDB services (mddbd, CLI)
#

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0

# Helper functions
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

# Cleanup function
cleanup() {
    echo -e "\n${YELLOW}Cleaning up...${NC}"
    pkill -f mddb-server 2>/dev/null || true
    rm -f /tmp/mddb-test.log
    rm -f mddb.db mddb.db.lock
    echo -e "${GREEN}Cleanup complete${NC}"
}

# Set trap to cleanup on exit
trap cleanup EXIT

# Start test suite
print_header "MDDB Authentication & Authorization Test Suite"

# Check if we're in the right directory
if [ ! -f "services/mddbd/main.go" ]; then
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

# Start server with auth enabled
print_header "Starting MDDB Server with Authentication"
export MDDB_AUTH_ENABLED=true
export MDDB_AUTH_JWT_SECRET=$(openssl rand -hex 32)
export MDDB_AUTH_ADMIN_USERNAME=admin
export MDDB_AUTH_ADMIN_PASSWORD=changeme

./mddb-server > /tmp/mddb-test.log 2>&1 &
SERVER_PID=$!
print_info "Server started with PID: $SERVER_PID"

# Wait for server to start
print_info "Waiting for server to start..."
sleep 3

if ! ps -p $SERVER_PID > /dev/null; then
    print_error "Server failed to start"
    cat /tmp/mddb-test.log
    exit 1
fi
print_success "Server is running"

# Return to root directory for tests
cd ../..

# ========================================
# Test 1: Unauthenticated Access
# ========================================
print_header "Test 1: Unauthenticated Access"
print_test "1" "Access /v1/stats without authentication (should fail with 401)"

HTTP_CODE=$(curl -s -w "%{http_code}" -o /tmp/response.json http://localhost:11023/v1/stats)
if [ "$HTTP_CODE" = "401" ]; then
    print_success "Correctly returned 401 Unauthorized"
    print_info "Response: $(cat /tmp/response.json)"
else
    print_error "Expected 401, got $HTTP_CODE"
fi

# ========================================
# Test 2: Login
# ========================================
print_header "Test 2: Login with Username & Password"
print_test "2" "Login as admin user"

LOGIN_RESPONSE=$(curl -s -H "Content-Type: application/json" \
    http://localhost:11023/v1/auth/login \
    -d '{"username":"admin","password":"changeme"}')

TOKEN=$(echo "$LOGIN_RESPONSE" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

if [ -n "$TOKEN" ]; then
    print_success "Login successful, received JWT token"
    print_info "Token: ${TOKEN:0:50}..."
    EXPIRES_AT=$(echo "$LOGIN_RESPONSE" | grep -o '"expiresAt":[0-9]*' | cut -d':' -f2)
    print_info "Expires at: $(date -r $EXPIRES_AT 2>/dev/null || date -d @$EXPIRES_AT 2>/dev/null)"
else
    print_error "Login failed, no token received"
    print_info "Response: $LOGIN_RESPONSE"
fi

# ========================================
# Test 3: Authenticated Access with JWT
# ========================================
print_header "Test 3: Authenticated Access with JWT Token"
print_test "3" "Access /v1/stats with valid JWT token (should succeed)"

HTTP_CODE=$(curl -s -w "%{http_code}" -o /tmp/response.json \
    -H "Authorization: Bearer $TOKEN" \
    http://localhost:11023/v1/stats)

if [ "$HTTP_CODE" = "200" ]; then
    print_success "Successfully accessed protected endpoint with JWT"
    print_info "Response: $(cat /tmp/response.json | head -c 100)..."
else
    print_error "Expected 200, got $HTTP_CODE"
fi

# ========================================
# Test 4: API Key Generation
# ========================================
print_header "Test 4: API Key Generation"
print_test "4" "Create API key"

API_KEY_RESPONSE=$(curl -s -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    http://localhost:11023/v1/auth/api-key \
    -d '{"description":"Test key for verification"}')

API_KEY=$(echo "$API_KEY_RESPONSE" | grep -o '"key":"[^"]*"' | cut -d'"' -f4)

if [[ "$API_KEY" =~ ^mddb_live_ ]]; then
    print_success "API key created successfully"
    print_info "Key format: ${API_KEY:0:20}... (${#API_KEY} chars)"
    print_info "Full response: $API_KEY_RESPONSE"
else
    print_error "API key generation failed or invalid format"
    print_info "Response: $API_KEY_RESPONSE"
fi

# ========================================
# Test 5: API Key Authentication
# ========================================
print_header "Test 5: API Key Authentication"
print_test "5" "Access /v1/stats with API key (should succeed)"

HTTP_CODE=$(curl -s -w "%{http_code}" -o /tmp/response.json \
    -H "X-API-Key: $API_KEY" \
    http://localhost:11023/v1/stats)

if [ "$HTTP_CODE" = "200" ]; then
    print_success "Successfully accessed endpoint with API key"
else
    print_error "Expected 200, got $HTTP_CODE"
fi

# ========================================
# Test 6: User Creation (RBAC)
# ========================================
print_header "Test 6: User Management"
print_test "6" "Create new user 'alice' (admin only)"

USER_RESPONSE=$(curl -s -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    http://localhost:11023/v1/auth/register \
    -d '{"username":"alice","password":"secret123"}')

if echo "$USER_RESPONSE" | grep -q "alice"; then
    print_success "User 'alice' created successfully"
    print_info "Response: $USER_RESPONSE"
else
    print_error "User creation failed"
    print_info "Response: $USER_RESPONSE"
fi

# ========================================
# Test 7: Set Permissions
# ========================================
print_header "Test 7: Set User Permissions"
print_test "7" "Grant Alice read-only permission to 'docs' collection"

PERM_RESPONSE=$(curl -s -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    http://localhost:11023/v1/auth/permissions \
    -d '{"username":"alice","collection":"docs","read":true,"write":false,"admin":false}')

if echo "$PERM_RESPONSE" | grep -q "ok"; then
    print_success "Permissions set successfully"
else
    print_error "Setting permissions failed"
    print_info "Response: $PERM_RESPONSE"
fi

# Get Alice's token
ALICE_TOKEN=$(curl -s -H "Content-Type: application/json" \
    http://localhost:11023/v1/auth/login \
    -d '{"username":"alice","password":"secret123"}' | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

print_info "Alice's token: ${ALICE_TOKEN:0:50}..."

# ========================================
# Test 8: Read Permission (Allowed)
# ========================================
print_header "Test 8: RBAC - Read Permission"
print_test "8" "Alice searches 'docs' collection (should succeed)"

HTTP_CODE=$(curl -s -w "%{http_code}" -o /tmp/response.json \
    -H "Authorization: Bearer $ALICE_TOKEN" \
    -H "Content-Type: application/json" \
    http://localhost:11023/v1/search \
    -d '{"collection":"docs","limit":10}')

if [ "$HTTP_CODE" = "200" ]; then
    print_success "Alice can read from 'docs' collection"
else
    print_error "Expected 200, got $HTTP_CODE"
fi

# ========================================
# Test 9: Write Permission (Denied)
# ========================================
print_header "Test 9: RBAC - Write Permission Denied"
print_test "9" "Alice tries to add document to 'docs' (should fail with 403)"

HTTP_CODE=$(curl -s -w "%{http_code}" -o /tmp/response.json \
    -H "Authorization: Bearer $ALICE_TOKEN" \
    -H "Content-Type: application/json" \
    http://localhost:11023/v1/add \
    -d '{"collection":"docs","key":"test","lang":"en","contentMd":"# Test"}')

if [ "$HTTP_CODE" = "403" ]; then
    print_success "Alice correctly denied write access (403 Forbidden)"
    print_info "Response: $(cat /tmp/response.json)"
else
    print_error "Expected 403, got $HTTP_CODE"
fi

# ========================================
# Test 10: Collection Isolation
# ========================================
print_header "Test 10: RBAC - Collection Isolation"
print_test "10" "Alice tries to access 'blog' collection (should fail with 403)"

HTTP_CODE=$(curl -s -w "%{http_code}" -o /tmp/response.json \
    -H "Authorization: Bearer $ALICE_TOKEN" \
    -H "Content-Type: application/json" \
    http://localhost:11023/v1/search \
    -d '{"collection":"blog","limit":10}')

if [ "$HTTP_CODE" = "403" ]; then
    print_success "Alice correctly denied access to 'blog' collection"
else
    print_error "Expected 403, got $HTTP_CODE"
fi

# ========================================
# Test 11: CLI Login
# ========================================
print_header "Test 11: CLI Service - Login"
print_test "11" "Test mddb-cli login command"

cd services/mddb-cli
CLI_OUTPUT=$(go run . login admin changeme 2>&1)

if echo "$CLI_OUTPUT" | grep -q "Login successful"; then
    print_success "CLI login command works"
    print_info "$(echo "$CLI_OUTPUT" | head -3)"
else
    print_error "CLI login failed"
    print_info "Output: $CLI_OUTPUT"
fi

# ========================================
# Test 12: CLI with Token
# ========================================
print_header "Test 12: CLI Service - Token Authentication"
print_test "12" "Test mddb-cli with --token flag"

CLI_OUTPUT=$(go run . --token $TOKEN stats 2>&1 | head -10)

if echo "$CLI_OUTPUT" | grep -q "MDDB Server Statistics"; then
    print_success "CLI works with JWT token"
    print_info "$(echo "$CLI_OUTPUT" | head -5)"
else
    print_error "CLI with token failed"
fi

# ========================================
# Test 13: CLI with API Key
# ========================================
print_header "Test 13: CLI Service - API Key Authentication"
print_test "13" "Test mddb-cli with --api-key flag"

CLI_OUTPUT=$(go run . --api-key $API_KEY stats 2>&1 | head -10)

if echo "$CLI_OUTPUT" | grep -q "MDDB Server Statistics"; then
    print_success "CLI works with API key"
else
    print_error "CLI with API key failed"
fi

cd ../..

# ========================================
# Final Summary
# ========================================
print_header "Test Summary"

TOTAL_TESTS=$((TESTS_PASSED + TESTS_FAILED))

echo -e "${BLUE}Total Tests:${NC} $TOTAL_TESTS"
echo -e "${GREEN}Passed:${NC} $TESTS_PASSED"
echo -e "${RED}Failed:${NC} $TESTS_FAILED"

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "\n${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}   ✓ ALL TESTS PASSED! Authentication is working correctly!${NC}"
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n"
    exit 0
else
    echo -e "\n${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${RED}   ✗ SOME TESTS FAILED - Please review the output above${NC}"
    echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n"
    exit 1
fi
