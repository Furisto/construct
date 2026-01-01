# Implement Token-Based Authentication System

## Overview

This PR implements a comprehensive token-based authentication system for Construct, enabling secure remote daemon access while maintaining backward compatibility with local Unix socket connections.

## Architecture

### Transport-Based Trust Model

Authentication is determined by connection transport:

- **Unix Socket**: Implicit admin trust via OS filesystem permissions - no token required
- **TCP**: Bearer token authentication with database validation and expiration checks

### Key Components

**Token Provider**
- Secure token generation using crypto/rand (256-bit entropy)
- SHA-256 hashing for storage (plaintext tokens never persisted)
- Setup code generation for secure token distribution (8-character codes, 20min expiry)
- In-memory setup code storage with single-use consumption

**Auth Interceptor**
- ConnectRPC middleware enforcing authentication on all endpoints
- Exempts `ExchangeSetupCode` for unauthenticated bootstrap
- Populates Identity context with subject, auth method, and privileges
- Validates token expiry and database presence

**AuthService RPCs**
- `CreateToken`: Generate new API tokens (admin only)
- `CreateSetupCode`: Generate short-lived exchange codes (admin only)
- `ListTokens`: Query tokens with filters (admin only)
- `RevokeToken`: Delete tokens by ID (admin only)
- `ExchangeSetupCode`: Bootstrap authentication without credentials (unauthenticated)

## Database Schema

New `Token` entity with Ent:
- `id`: UUID primary key
- `name`: Unique human-readable identifier
- `type`: Enum (api_token | setup_code)
- `token_hash`: SHA-256 hash for validation
- `description`: Optional metadata
- `expires_at`: Expiration timestamp
- `created_at` / `updated_at`: Automatic timestamps

Indexes on `name` and `token_hash` for efficient lookups.

## Security Features

✅ Cryptographically secure token generation (crypto/rand)
✅ Only SHA-256 hashes stored in database
✅ Plaintext tokens shown once at creation
✅ Configurable token expiration (default 90 days, max 365 days)
✅ Setup codes expire quickly (default 20 minutes, max 72 hours)
✅ Single-use setup codes (deleted after consumption)
✅ Case-insensitive setup codes for usability
✅ Thread-safe setup code storage with mutex
✅ Admin-only token management operations

## Setup Code Flow

Designed for secure token distribution without exposing tokens:

1. Admin runs `construct daemon token create --setup-code laptop` via Unix socket
2. Daemon generates short code like `ABCD-1234`, stores in memory (20min TTL)
3. Admin shares code out-of-band (verbally, Slack, etc.)
4. User runs `construct context add prod --endpoint https://... --setup-code ABCD-1234`
5. Client calls `ExchangeSetupCode` RPC (unauthenticated)
6. Daemon validates code, generates real token, deletes code
7. Client receives token, stores in system keyring

## Testing

**Unit Tests** (`backend/api/auth/token_test.go`)
- Token generation with format validation
- Consistent hashing
- Setup code creation and expiration
- Single-use consumption
- Case-insensitive handling

**Integration Tests** (`backend/api/auth_test.go`)
- All AuthService RPCs with success and error cases
- Admin authorization enforcement
- Duplicate name detection
- Token filtering and expiry handling
- Setup code validation

**Test Infrastructure Updates**
- Added TokenProvider to test handler options
- Token table cleanup between test runs
- Generated AuthServiceClient mock

All tests pass and compile successfully.

## Changes

### New Files
- `backend/memory/schema/token.go` - Token entity definition
- `backend/memory/schema/types/token.go` - TokenType enum
- `backend/api/auth/identity.go` - Identity and AuthMethod types
- `backend/api/auth/transport.go` - Transport context utilities
- `backend/api/auth/token.go` - TokenProvider implementation
- `backend/api/auth/interceptor.go` - Authentication interceptor
- `backend/api/auth.go` - AuthService handler
- `backend/api/auth/token_test.go` - TokenProvider unit tests
- `backend/api/auth_test.go` - AuthService integration tests
- `api/go/client/mocks/auth.connect_mock.go` - Generated mock

### Modified Files
- `backend/api/api.go` - Integrated auth interceptor and transport context
- `backend/api/api_test.go` - Added TokenProvider and Token cleanup
- `api/go/client/client.go` - Added Auth() method

### Generated Files
- `backend/memory/*` - Ent-generated code for Token entity
- `api/go/v1/v1connect/auth.connect.go` - ConnectRPC service handlers (already existed)

## Backward Compatibility

✅ Local Unix socket usage unchanged (no auth required)
✅ Existing commands continue to work without modification
✅ No breaking changes to API or CLI

## Future Work (Out of Scope)

- OIDC/SSO integration
- Token scopes and permissions
- Token rotation with refresh tokens
- Multi-user support
- Team/organization management

## Commits

All commits follow project conventions with co-author attribution.
