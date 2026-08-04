---
title: "Authentication Quick Start"
slug: "docs/auth-quickstart"
description: "Turn on MDDB authentication in five minutes: create the first admin user, issue a JWT, and lock a collection down with role-based permissions."
status: publish
---

# Authentication Quick Start

5-minute guide to get started with MDDB authentication.

## Start with Auth Enabled

```bash
cd services/mddbd

MDDB_AUTH_ENABLED=true \
MDDB_AUTH_JWT_SECRET=$(openssl rand -hex 32) \
MDDB_AUTH_ADMIN_USERNAME=admin \
MDDB_AUTH_ADMIN_PASSWORD=changeme \
go run .
```

## Login

```bash
curl http://localhost:11023/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"changeme"}'
```

Save the token from the response.

## Use Token

```bash
TOKEN="your-token-here"

# View stats
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:11023/v1/stats

# Add document
curl -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  http://localhost:11023/v1/add \
  -d '{
    "collection": "docs",
    "key": "welcome",
    "lang": "en",
    "contentMd": "# Welcome to MDDB"
  }'
```

## Create API Key

```bash
curl -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  http://localhost:11023/v1/auth/api-key \
  -d '{"description":"My API key"}'
```

Save the API key (shown only once!).

## Use API Key

```bash
curl -H "X-API-Key: mddb_live_..." \
  http://localhost:11023/v1/stats
```

## Create User with Limited Access

```bash
# Create user
curl -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  http://localhost:11023/v1/auth/register \
  -d '{"username":"alice","password":"secret123"}'

# Grant read-only to "blog" collection
curl -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  http://localhost:11023/v1/auth/permissions \
  -d '{
    "username": "alice",
    "collection": "blog",
    "read": true,
    "write": false,
    "admin": false
  }'
```

## Test Suite

Run automated tests:

```bash
./test-auth.sh    # Core authentication and RBAC
./test-mcp.sh     # MCP service integration
./test-panel.sh   # Panel UI (manual browser test)
```

## Full Documentation

See [AUTHENTICATION.md](AUTHENTICATION.md) for complete documentation.

## Need Help?

- Authentication not working? Check `MDDB_AUTH_JWT_SECRET` is set
- Getting 401? Make sure you include `Authorization: Bearer TOKEN` header
- Getting 403? User needs permissions - check with `/v1/auth/permissions`
- API key not working? Use `X-API-Key` header (not `Authorization`)

## Disable Authentication

Set `MDDB_AUTH_ENABLED=false` or omit it (disabled by default).
