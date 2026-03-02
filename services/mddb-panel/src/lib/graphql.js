import { createClient, cacheExchange, fetchExchange } from 'urql';

// GraphQL client instance
let graphqlClient = null;

/**
 * Get or create GraphQL client
 * @returns {Client} urql client instance
 */
export function getGraphQLClient() {
  if (!graphqlClient) {
    const serverUrl = import.meta.env.VITE_SERVER_URL || 'http://localhost:11023';

    graphqlClient = createClient({
      url: `${serverUrl}/graphql`,
      exchanges: [cacheExchange, fetchExchange],
      fetchOptions: () => {
        const token = localStorage.getItem('token');
        const apiKey = localStorage.getItem('apiKey');

        const headers = {
          'Content-Type': 'application/json',
        };

        if (token) {
          headers['Authorization'] = `Bearer ${token}`;
        } else if (apiKey) {
          headers['X-API-Key'] = apiKey;
        }

        return { headers };
      },
    });
  }

  return graphqlClient;
}

/**
 * Execute a GraphQL query
 * @param {string} query - GraphQL query string
 * @param {object} variables - Query variables
 * @returns {Promise<object>} Query result
 */
export async function executeQuery(query, variables = {}) {
  const client = getGraphQLClient();
  const result = await client.query(query, variables).toPromise();

  if (result.error) {
    throw new Error(result.error.message);
  }

  return result.data;
}

/**
 * Execute a GraphQL mutation
 * @param {string} mutation - GraphQL mutation string
 * @param {object} variables - Mutation variables
 * @returns {Promise<object>} Mutation result
 */
export async function executeMutation(mutation, variables = {}) {
  const client = getGraphQLClient();
  const result = await client.mutation(mutation, variables).toPromise();

  if (result.error) {
    throw new Error(result.error.message);
  }

  return result.data;
}

// GraphQL query templates
export const QUERIES = {
  // Stats
  GET_STATS: `
    query GetStats {
      stats {
        totalDocuments
        totalRevisions
        databaseSize
        databasePath
        mode
        collections {
          name
          documentCount
          revisionCount
        }
      }
    }
  `,

  // Document operations
  GET_DOCUMENT: `
    query GetDocument($collection: String!, $key: String!, $lang: String!, $env: JSONObject) {
      document(collection: $collection, key: $key, lang: $lang, env: $env) {
        id
        key
        lang
        meta
        contentMd
        addedAt
        updatedAt
        expiresAt
      }
    }
  `,

  SEARCH_DOCUMENTS: `
    query SearchDocuments($input: SearchInput!) {
      search(input: $input) {
        edges {
          cursor
          node {
            id
            key
            lang
            meta
            contentMd
            addedAt
            updatedAt
          }
        }
        pageInfo {
          hasNextPage
          hasPreviousPage
          totalCount
        }
        totalCount
      }
    }
  `,

  // Vector search
  VECTOR_SEARCH: `
    query VectorSearch($input: VectorSearchInput!) {
      vectorSearch(input: $input) {
        results {
          document {
            id
            key
            lang
            meta
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
  `,

  VECTOR_STATS: `
    query VectorStats {
      vectorStats {
        enabled
        provider
        model
        dimensions
        indexReady
        collections
      }
    }
  `,

  // Auth
  ME: `
    query Me {
      me {
        username
        admin
        createdAt
      }
    }
  `,
};

export const MUTATIONS = {
  // Auth
  LOGIN: `
    mutation Login($username: String!, $password: String!) {
      login(username: $username, password: $password) {
        token
        expiresAt
      }
    }
  `,

  // Document operations
  ADD_DOCUMENT: `
    mutation AddDocument($input: AddDocumentInput!) {
      addDocument(input: $input) {
        id
        key
        lang
        addedAt
        updatedAt
      }
    }
  `,

  UPDATE_DOCUMENT: `
    mutation UpdateDocument($input: UpdateDocumentInput!) {
      updateDocument(input: $input) {
        id
        updatedAt
      }
    }
  `,

  DELETE_DOCUMENT: `
    mutation DeleteDocument($collection: String!, $key: String!, $lang: String!) {
      deleteDocument(collection: $collection, key: $key, lang: $lang)
    }
  `,

  DELETE_COLLECTION: `
    mutation DeleteCollection($collection: String!) {
      deleteCollection(collection: $collection)
    }
  `,

  // Vector operations
  VECTOR_REINDEX: `
    mutation VectorReindex($collection: String, $force: Boolean) {
      vectorReindex(collection: $collection, force: $force) {
        enabled
        collections
      }
    }
  `,
};
