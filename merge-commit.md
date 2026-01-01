Add token-based authentication for remote daemon access

Implement authentication system enabling secure connections to remote
Construct daemons while preserving local Unix socket simplicity.

Core authentication flows:
- Unix socket connections: implicit admin via OS permissions
- TCP connections: Bearer token validation with database lookup
- Setup codes: secure bootstrap mechanism for token distribution

Token security model:
- 256-bit cryptographic randomness (crypto/rand)
- SHA-256 hashing (plaintext never stored)
- Configurable expiration (90 day default, 365 day max)
- One-time display at creation

Setup code bootstrap:
- Short-lived codes (20 minute default expiry)
- Single-use consumption with automatic deletion
- Case-insensitive for usability
- Thread-safe in-memory storage

Database additions:
- Token entity via Ent schema
- Fields: id, name, type, token_hash, description, expires_at
- Unique indexes on name and token_hash
- TokenType enum: api_token, setup_code

API additions:
- AuthService with 5 RPCs (CreateToken, CreateSetupCode, ListTokens, 
  RevokeToken, ExchangeSetupCode)
- ConnectRPC interceptor for authentication enforcement
- ExchangeSetupCode exempt from auth (bootstrap path)
- Transport context injection (unix vs tcp detection)

Client updates:
- Auth() accessor for AuthServiceClient
- Mock generation for testing
- Bearer token injection via interceptor

Testing:
- Unit tests for token generation, hashing, setup codes
- Integration tests for all AuthService operations
- Admin authorization enforcement validation
- Test infrastructure extended with TokenProvider

This enables remote daemon deployment while maintaining zero-config
local usage. Foundation for future OIDC integration.
