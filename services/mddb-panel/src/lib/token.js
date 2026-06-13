// Single source of truth for the panel's auth-token storage key and JWT shape
// validation (FE-005). Kept free of browser APIs so it is unit-testable with
// `node --test` and can be imported by both auth.js and graphql.js without the
// two drifting apart again.

// The one key under which the JWT is stored. graphql.js previously read the
// dead keys 'token' / 'apiKey', so the Authorization header was never set.
export const TOKEN_KEY = 'mddb_auth_token';

// Legacy keys cleared on startup (older builds / accidental writes).
export const LEGACY_TOKEN_KEYS = ['token', 'apiKey'];

const JWT_SHAPE = /^[\w-]+\.[\w-]+\.[\w-]+$/;

// isValidJwtShape rejects obviously-malformed tokens before they are attached
// to a request (header.payload.signature, base64url segments only).
export function isValidJwtShape(token) {
  return typeof token === 'string' && JWT_SHAPE.test(token);
}
