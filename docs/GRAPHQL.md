# MDDB GraphQL API Documentation

## Overview

MDDB provides a modern GraphQL API alongside its REST and gRPC interfaces. The GraphQL endpoint offers flexible querying, type safety, and schema introspection.

**Endpoint:** `POST /graphql`
**Playground:** `GET /playground` (development tool)

## Quick Start

### Enable GraphQL

GraphQL is **disabled by default**. Enable it via environment variable or CLI flag:

```bash
# Environment variable
export MDDB_GRAPHQL_ENABLED=true
./mddbd

# CLI flag
./mddbd --graphql

# Docker
docker run -e MDDB_GRAPHQL_ENABLED=true tradik/mddb
```

### Access GraphQL Playground

Visit `http://localhost:11023/playground` to explore the API interactively.

## Authentication

GraphQL uses the same JWT/API key authentication as the REST API.

### Login Mutation

```graphql
mutation {
  login(username: "admin", password: "your-password") {
    token
    expiresAt
  }
}
```

### Using the Token

Include the token in subsequent requests:

**HTTP Header:**
```
Authorization: Bearer YOUR_JWT_TOKEN
```

**Example with curl:**
```bash
curl -X POST http://localhost:11023/graphql \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"query": "{ me { username admin } }"}'
```

### API Key Authentication

Alternatively, use an API key:

```
X-API-Key: mddb_live_YOUR_API_KEY
```

## Schema Overview

### Core Types

#### Document
```graphql
type Document {
  id: ID!
  key: String!
  lang: String!
  meta: JSONObject!
  contentMd: String!
  addedAt: Time!
  updatedAt: Time!
  expiresAt: Time
}
```

#### VectorSearchResult
```graphql
type VectorSearchResult {
  document: Document!
  score: Float!
  rank: Int!
}
```

#### User
```graphql
type User {
  username: String!
  admin: Boolean!
  createdAt: Time!
}
```

### Authentication Directives

- `@auth` - Requires authentication (JWT or API key)
- `@hasRole(role: Role!)` - Requires specific role (ADMIN or USER)

## Queries

### Get Current User

```graphql
query {
  me {
    username
    admin
    createdAt
  }
}
```

### Get Single Document

```graphql
query {
  document(
    collection: "blog"
    key: "hello-world"
    lang: "en"
  ) {
    id
    key
    lang
    contentMd
    meta
    addedAt
    updatedAt
  }
}
```

**With Template Variables:**
```graphql
query {
  document(
    collection: "templates"
    key: "email"
    lang: "en"
    env: { name: "John", year: "2024" }
  ) {
    contentMd
  }
}
```

### Search Documents

```graphql
query {
  search(input: {
    collection: "blog"
    filterMeta: [
      { key: "status", values: ["published"] }
      { key: "author", values: ["John", "Jane"] }
    ]
    sort: "updatedAt"
    asc: false
    limit: 10
    offset: 0
  }) {
    edges {
      cursor
      node {
        key
        lang
        contentMd
        meta
      }
    }
    pageInfo {
      hasNextPage
      hasPreviousPage
      totalCount
    }
  }
}
```

### Vector Search (Semantic Search)

```graphql
query {
  vectorSearch(input: {
    collection: "kb"
    query: "how to configure authentication?"
    topK: 5
    threshold: 0.7
    includeContent: true
  }) {
    results {
      document {
        key
        lang
        contentMd
      }
      score
      rank
    }
    total
    model
    dimensions
  }
}
```

**With Pre-computed Vector:**
```graphql
query {
  vectorSearch(input: {
    collection: "kb"
    queryVector: [0.1, 0.2, 0.3, ...]  # 1536-dim for OpenAI
    topK: 10
  }) {
    results {
      document { key contentMd }
      score
    }
  }
}
```

### Full-Text Search

```graphql
query {
  fts(input: {
    collection: "docs"
    query: "authentication jwt token"
    limit: 20
  }) {
    results {
      document {
        key
        contentMd
      }
      score
      matchedTerms
    }
    total
  }
}
```

### Database Statistics

```graphql
query {
  stats {
    databasePath
    databaseSize
    mode
    totalDocuments
    totalRevisions
    collections {
      name
      documentCount
      revisionCount
    }
  }
}
```

### Vector Statistics

```graphql
query {
  vectorStats {
    enabled
    provider
    model
    dimensions
    indexReady
    collections
  }
}
```

## Mutations

### Login

```graphql
mutation {
  login(username: "admin", password: "secret") {
    token
    expiresAt
  }
}
```

### Add Document

```graphql
mutation {
  addDocument(input: {
    collection: "blog"
    key: "my-post"
    lang: "en"
    meta: [
      { key: "author", values: ["John"] }
      { key: "tags", values: ["tutorial", "graphql"] }
      { key: "status", values: ["published"] }
    ]
    contentMd: """
    # My Blog Post

    This is the content of my post.
    """
    ttl: 3600  # optional: expires in 1 hour
  }) {
    id
    key
    addedAt
  }
}
```

### Update Document

```graphql
mutation {
  updateDocument(input: {
    collection: "blog"
    key: "my-post"
    lang: "en"
    meta: [
      { key: "status", values: ["draft"] }
    ]
    contentMd: "# Updated Content"
  }) {
    id
    updatedAt
  }
}
```

### Delete Document

```graphql
mutation {
  deleteDocument(
    collection: "blog"
    key: "my-post"
    lang: "en"
  )
}
```

### Delete Collection

```graphql
mutation {
  deleteCollection(collection: "old-blog")
}
```

### Reindex Vectors

```graphql
mutation {
  vectorReindex(
    collection: "kb"
    force: false
  ) {
    enabled
    collections
  }
}
```

## Admin Operations

### Register User (Admin Only)

```graphql
mutation {
  register(
    username: "newuser"
    password: "secure-password"
  ) {
    username
    admin
    createdAt
  }
}
```

### Create API Key

```graphql
mutation {
  createAPIKey(input: {
    description: "My API key"
    expiresAt: 1735689600  # Unix timestamp, optional
  }) {
    key
    description
    expiresAt
    createdAt
  }
}
```

### Create Group

```graphql
mutation {
  createGroup(input: {
    name: "editors"
    description: "Content editors"
    members: ["user1", "user2"]
  }) {
    name
    description
    members
  }
}
```

### Set Permission

```graphql
mutation {
  setPermission(input: {
    username: "john"
    collection: "blog"
    read: true
    write: true
    admin: false
  })
}
```

## Advanced Usage

### Combining Multiple Queries

GraphQL allows fetching multiple resources in one request:

```graphql
query {
  currentUser: me {
    username
    admin
  }

  recentPosts: search(input: {
    collection: "blog"
    sort: "updatedAt"
    limit: 5
  }) {
    edges {
      node {
        key
        contentMd
      }
    }
  }

  dbStats: stats {
    totalDocuments
    databaseSize
  }
}
```

### Field Selection

Request only the fields you need:

```graphql
query {
  document(collection: "blog", key: "post", lang: "en") {
    key        # Only these 2 fields returned
    contentMd
  }
}
```

### Variables

Use GraphQL variables for dynamic queries:

```graphql
query GetDocument($coll: String!, $key: String!, $lang: String!) {
  document(collection: $coll, key: $key, lang: $lang) {
    id
    contentMd
  }
}
```

**Variables:**
```json
{
  "coll": "blog",
  "key": "my-post",
  "lang": "en"
}
```

## Error Handling

GraphQL returns structured errors:

```json
{
  "errors": [
    {
      "message": "authentication failed: invalid credentials",
      "path": ["login"]
    }
  ],
  "data": null
}
```

Common error messages:
- `"unauthorized: authentication required"` - Missing or invalid token
- `"permission denied"` - Insufficient permissions
- `"not yet implemented - use REST API"` - Feature pending implementation
- `"authentication failed"` - Invalid credentials

## Performance Considerations

### Query Complexity

MDDB limits query complexity to prevent resource exhaustion:
- Max complexity: 1000 points
- Max depth: 10 levels

### Pagination

Use cursor-based pagination for large result sets:

```graphql
query {
  search(input: { collection: "blog", limit: 50 }) {
    edges {
      cursor
      node { key }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
```

### Caching

Parsed GraphQL queries are cached in memory (LRU cache).

## GraphQL vs REST vs gRPC

| Feature | GraphQL | REST | gRPC |
|---------|---------|------|------|
| Field selection | ✅ Flexible | ❌ Fixed | ❌ Fixed |
| Multiple resources | ✅ Single request | ❌ Multiple requests | ✅ Streaming |
| Type safety | ✅ Schema | ❌ None | ✅ Protobuf |
| Introspection | ✅ Built-in | ❌ None | ✅ Reflection |
| Binary protocol | ❌ JSON | ❌ JSON | ✅ Protobuf |
| Streaming | ⚠️ Limited | ❌ No | ✅ Yes |
| Simplicity | ⚠️ Learning curve | ✅ Simple | ⚠️ Setup |

**Use GraphQL when:**
- You need flexible field selection
- You want to combine multiple queries
- You're building a modern frontend (React, Vue, Angular)
- You prefer strong typing and schema validation

**Use REST when:**
- You need simple curl/wget access
- You're building quick scripts or automations
- You need streaming responses (export, backup)

**Use gRPC when:**
- You need maximum performance
- You're building service-to-service communication
- You need bidirectional streaming

## Configuration

### Environment Variables

- `MDDB_GRAPHQL_ENABLED` - Enable/disable GraphQL (default: `false`)
- `MDDB_GRAPHQL_PLAYGROUND` - Enable/disable Playground (default: `true`)
- `MDDB_AUTH_ENABLED` - Enable authentication (default: `false`)
- `MDDB_AUTH_JWT_SECRET` - JWT secret (required if auth enabled)

### CLI Flags

```bash
./mddbd --graphql                    # Enable GraphQL
./mddbd --graphql --playground=false # Disable Playground
```

## Client Libraries

### JavaScript/TypeScript

```bash
npm install @apollo/client graphql
```

```typescript
import { ApolloClient, InMemoryCache, gql } from '@apollo/client';

const client = new ApolloClient({
  uri: 'http://localhost:11023/graphql',
  cache: new InMemoryCache(),
  headers: {
    authorization: `Bearer ${token}`
  }
});

const { data } = await client.query({
  query: gql`
    query {
      stats {
        totalDocuments
      }
    }
  `
});
```

### Python

```bash
pip install gql
```

```python
from gql import gql, Client
from gql.transport.requests import RequestsHTTPTransport

transport = RequestsHTTPTransport(
    url='http://localhost:11023/graphql',
    headers={'Authorization': f'Bearer {token}'}
)

client = Client(transport=transport, fetch_schema_from_transport=True)

query = gql('''
    query {
        stats {
            totalDocuments
        }
    }
''')

result = client.execute(query)
```

## Troubleshooting

### GraphQL endpoint not responding

Check if GraphQL is enabled:
```bash
curl http://localhost:11023/v1/config | jq .graphql_enabled
```

### Authentication errors

Verify your token:
```bash
curl -X POST http://localhost:11023/graphql \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"query": "{ me { username } }"}'
```

### Schema introspection

Query the schema directly:
```graphql
query {
  __schema {
    types {
      name
    }
  }
}
```

## Future Enhancements

The following features are planned for future releases:

- Full document CRUD operations via GraphQL
- Subscriptions for real-time updates
- Batch operations (bulk add/delete)
- Advanced filtering with logical operators
- Schema validation via GraphQL
- Rate limiting per user/IP
- DataLoader for N+1 query prevention

## Contributing

GraphQL resolver implementations are in progress. Contributions welcome:
- `services/mddbd/graphql/schema.resolvers.go` - Resolver implementations
- `services/mddbd/graphql_adapter.go` - Server adapter

## Support

- GitHub Issues: https://github.com/tradik/mddb/issues
- Documentation: https://github.com/tradik/mddb/tree/main/docs
- REST API Docs: `/v1/endpoints`

---

**Version:** 2.6.6
**Last Updated:** March 2026
