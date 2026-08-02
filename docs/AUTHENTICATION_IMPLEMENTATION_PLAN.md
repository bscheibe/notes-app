# Authentication Implementation Plan

## Requirements Analysis

### Functional Requirements
1. **Authentication Page**: Display login options with federation buttons
2. **Google OAuth**: Allow users to authenticate via Google account
3. **GitHub OAuth**: Allow users to authenticate via GitHub account
4. **Guest Access**: Allow users to continue as guest with session-only data
5. **Session Management**: Maintain authentication state across requests
6. **Route Protection**: Secure routes that require authentication

### Non-Functional Requirements
- **Security**: Industry-standard OAuth2 flow, secure session handling
- **Testability**: Each component independently testable
- **Scalability**: Support for multiple auth providers (Google, GitHub, future providers)
- **User Experience**: Seamless login flow, clear guest vs authenticated states

## Architecture Overview

### Component Diagram
```
┌─────────────────┐
│   Auth Handler  │  ← Login page, OAuth callbacks, guest login
└────────┬────────┘
         │
┌────────▼────────┐
│  Auth Service   │  ← Business logic, session management
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
┌───▼───┐ ┌──▼────┐
│ OAuth │ │Session│
│ Client│ │Store  │
└───────┘ └───────┘
```

## Major Components & Technologies

### 1. OAuth2 Integration
**Technology**: `golang.org/x/oauth2` + `golang.org/x/oauth2/google` + `golang.org/x/oauth2/github`

**Why This Choice**:
- **Official Go OAuth2 implementation**: Maintained by Go team, battle-tested
- **Provider-specific packages**: Pre-configured for Google and GitHub OAuth endpoints
- **Industry standard**: OAuth2 is the de facto standard for federation
- **Security**: Handles token exchange, PKCE, and secure redirects properly
- **Flexibility**: Easy to add other providers (Microsoft, Facebook, etc.)

**Implementation Notes**:
- Handles the OAuth2 authorization code flow for multiple providers
- Manages state parameter to prevent CSRF attacks
- Token storage and refresh handling
- User profile retrieval from Google and GitHub
- Provider abstraction layer for consistent API across providers

**GitHub OAuth Specifics**:
- GitHub uses the same OAuth2 standard as Google but with different endpoints
- GitHub provides developer-focused authentication with access to repositories, Gists, etc.
- GitHub OAuth returns user data including username, avatar URL, and email
- Scopes can be customized (e.g., `user:email` for basic profile, `repo` for repository access)
- GitHub's OAuth is particularly valuable for developer tools and version control integration

### 2. Session Management
**Technology**: `github.com/gorilla/sessions`

**Why This Choice**:
- **Industry standard**: Most widely used session library in Go ecosystem
- **Secure defaults**: Automatic CSRF protection, secure cookie flags
- **Flexible backends**: Cookie-based, file system, Redis, etc.
- **Mature battle-tested**: Used in production by thousands of applications
- **Simple API**: Easy integration with existing middleware

**Implementation Notes**:
- Cookie-based sessions for simplicity (can upgrade to Redis later)
- Separate session stores for authenticated vs guest users
- Session expiration and cleanup
- Secure cookie configuration (HttpOnly, Secure, SameSite)

### 3. User/Identity Models
**Technology**: Custom Go structs with interface abstraction

**Why This Choice**:
- **Go idiomatic**: Unlike frameworks in other languages (Spring Security, Django Auth), Go favors custom domain models
- **Type safety**: Compile-time validation of user data
- **Interface abstraction**: Easy to mock for testing
- **No framework lock-in**: Pure Go, no ORM dependencies initially
- **Domain-specific**: Your federation requirements (Google + GitHub + guest) don't fit generic user models
- **Simple storage**: Can use file system initially, upgrade to database later

**Go Philosophy Note**: In Go, custom user models are the standard approach. Unlike Java/Spring or Django, Go doesn't provide framework-level user models. Go developers prefer:
- Domain-specific structs over generic abstractions
- Libraries for specific concerns (JWT, OAuth) rather than complete auth frameworks
- Explicit design over convention-over-configuration

**When to use complete identity solutions**: If you were building a full identity provider (like Auth0), you'd use Ory Kratos. For consuming OAuth2 (your use case), custom models are appropriate.

**Implementation Notes**:
- `User` struct for OAuth users (persistent)
- `GuestSession` struct for guest users (ephemeral)
- `Identity` interface for polymorphic handling
- Profile data from OAuth providers (name, email, avatar)
- Provider field to distinguish between Google and GitHub users

### 4. Authentication Middleware
**Technology**: Custom Chi middleware + session integration

**Why This Choice**:
- **Chi integration**: Consistent with existing router choice
- **Middleware pattern**: Standard Go web practice
- **Composable**: Can chain with existing middleware
- **Testable**: Easy to unit test middleware logic

**Implementation Notes**:
- `RequireAuth` middleware for protected routes
- `OptionalAuth` middleware for routes that work with or without auth
- User context injection for handlers
- Guest vs authenticated user differentiation

### 5. Configuration Management
**Technology**: Extend existing Viper configuration

**Why This Choice**:
- **Consistency**: Already using Viper in the project
- **Environment-specific**: Different OAuth credentials per environment
- **Secrets handling**: OAuth secrets via environment variables
- **No additional dependencies**: Leverages existing infrastructure

**Implementation Notes**:
- OAuth client ID and secret configuration for multiple providers
- Session cookie settings (name, duration, secrets)
- Redirect URLs for OAuth callbacks (per provider)
- Guest session settings
- Provider-specific configuration (scopes, endpoints)

## Implementation Task List

### Phase 1: Foundation (Testable in Isolation)

#### Task 1.1: Create User and Identity Models
**Files**: `internal/models/user.go`, `internal/models/identity.go`
**Tests**: `internal/models/user_test.go`
**Description**:
- Define `User` struct for OAuth users
- Define `GuestSession` struct for temporary sessions
- Create `Identity` interface for polymorphic handling
- Add `Provider` enum for Google, GitHub, and future providers
- Add validation methods
**Success Criteria**: Models compile, validation tests pass

#### Task 1.2: Create Session Store Abstraction
**Files**: `internal/auth/session_store.go`, `internal/auth/session_store_test.go`
**Description**:
- Create interface for session operations
- Implement file system-based session store
- Add session creation, retrieval, deletion
- Add session expiration logic
**Success Criteria**: Session store tests pass, can create/retrieve/delete sessions

#### Task 1.3: Create User Repository
**Files**: `internal/auth/user_repository.go`, `internal/auth/user_repository_test.go`
**Description**:
- Create interface for user operations
- Implement file system-based user storage
- Add user creation, retrieval by ID/email
- Add user profile updates
**Success Criteria**: User repository tests pass, can store and retrieve users

#### Task 1.4: Extend Configuration
**Files**: `internal/config/config.go`
**Description**:
- Add OAuth configuration fields for Google and GitHub (client ID, secret, callback URL)
- Add session configuration (cookie name, duration, secret)
- Add environment variable mappings for both providers
- Update config files with example values
**Success Criteria**: Config loads correctly with new fields

### Phase 2: Authentication Service (Business Logic)

#### Task 2.1: Create Authentication Service
**Files**: `internal/auth/auth_service.go`, `internal/auth/auth_service_test.go`
**Description**:
- Implement Google OAuth flow initiation
- Implement GitHub OAuth flow initiation
- Implement OAuth callback handling (provider-agnostic)
- Implement guest session creation
- Implement session validation
- Implement user creation/retrieval from OAuth data
**Success Criteria**: Auth service tests pass, can initiate OAuth for both providers, handle callbacks, create guest sessions

#### Task 2.2: Create OAuth Client Wrapper
**Files**: `internal/auth/oauth_client.go`, `internal/auth/oauth_client_test.go`
**Description**:
- Wrap `golang.org/x/oauth2` for testability
- Implement Google OAuth configuration
- Implement GitHub OAuth configuration
- Add token exchange logic (provider-agnostic)
- Add user profile retrieval from Google and GitHub
**Success Criteria**: OAuth client tests pass, can configure and use OAuth2 client for both providers

### Phase 3: HTTP Layer (Handlers & Middleware)

#### Task 3.1: Create Authentication Handlers
**Files**: `internal/handlers/auth_handler.go`, `internal/handlers/auth_handler_test.go`
**Description**:
- Create login page handler (shows OAuth buttons + guest option)
- Create Google OAuth redirect handler
- Create GitHub OAuth redirect handler
- Create OAuth callback handler (provider-agnostic)
- Create guest login handler
- Create logout handler
**Success Criteria**: Handler tests pass, login page renders, OAuth redirects work for both providers

#### Task 3.2: Create Authentication Middleware
**Files**: `internal/middleware/auth_middleware.go`, `internal/middleware/auth_middleware_test.go`
**Description**:
- Implement `RequireAuth` middleware
- Implement `OptionalAuth` middleware
- Add user context injection
- Add guest vs authenticated user handling
**Success Criteria**: Middleware tests pass, protected routes require auth, optional routes work both ways

#### Task 3.3: Integrate with Existing Routes
**Files**: `internal/server/server.go`
**Description**:
- Add authentication routes to router
- Apply middleware to existing note routes
- Update handlers to use user context
- Add login link to existing pages
**Success Criteria**: Server starts, authentication flow works end-to-end

### Phase 4: Frontend Integration

#### Task 4.1: Create Login Page Template
**Files**: `templates/login.html`
**Description**:
- Create login page with Google and GitHub OAuth buttons
- Add guest login option
- Style to match existing design
- Add error display for failed auth
**Success Criteria**: Login page renders correctly, buttons are clickable

#### Task 4.2: Update Existing Templates
**Files**: `templates/index.html`, etc.
**Description**:
- Add user info display (name, avatar)
- Add logout button when authenticated
- Add login link when not authenticated
- Show guest vs authenticated state
**Success Criteria**: Templates show correct auth state, logout works

### Phase 5: Testing & Refinement

#### Task 5.1: Integration Tests
**Files**: `tests/integration/auth_test.go`
**Description**:
- Test complete Google OAuth flow (mocked)
- Test complete GitHub OAuth flow (mocked)
- Test guest login flow
- Test protected route access
- Test session persistence
**Success Criteria**: Integration tests pass, end-to-end flows work for both providers

#### Task 5.2: Security Hardening
**Files**: Multiple files as needed
**Description**:
- Add CSRF protection
- Add session fixation prevention
- Add secure cookie configuration
- Add OAuth state parameter validation
- Add rate limiting on auth endpoints
**Success Criteria**: Security checks pass, no obvious vulnerabilities

#### Task 5.3: Documentation
**Files**: `README.md`, `docs/AUTHENTICATION_ARCHITECTURE.md`
**Description**:
- Document OAuth setup process for Google and GitHub
- Document configuration options for multiple providers
- Document testing approach
- Add OAuth credentials setup guide for both providers
**Success Criteria**: Documentation is complete and accurate

## Technology Choices Summary

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| OAuth2 | `golang.org/x/oauth2` | Official Go implementation, industry standard |
| Google OAuth | `golang.org/x/oauth2/google` | Pre-configured Google endpoints |
| GitHub OAuth | `golang.org/x/oauth2/github` | Pre-configured GitHub endpoints |
| Sessions | `github.com/gorilla/sessions` | Industry standard, secure defaults |
| Storage | File system (extendable) | Simple start, easy to upgrade to DB |
| Router | Chi (existing) | Consistent with current architecture |
| Config | Viper (existing) | Consistent with current architecture |
| Testing | Standard Go testing + testify | Consistent with current approach |

## Testing Strategy

### Unit Tests
- Models: Validate struct fields and methods
- Repositories: Test CRUD operations with file system
- Services: Test business logic with mocked dependencies
- Handlers: Test HTTP responses with httptest
- Middleware: Test request flow and context injection

### Integration Tests
- Complete Google OAuth flow (with mocked Google responses)
- Complete GitHub OAuth flow (with mocked GitHub responses)
- Guest login flow
- Protected route access
- Session management across requests

### Test Data Management
- Use temporary directories for file system storage
- Clean up test data after each test
- Use test-specific configuration
- Mock external dependencies (Google API)

## Incremental Build Strategy

Each phase builds on the previous:
1. **Phase 1**: Foundation components (models, repositories) - no external dependencies
2. **Phase 2**: Business logic (auth service) - uses Phase 1 components
3. **Phase 3**: HTTP layer (handlers, middleware) - uses Phase 2 components
4. **Phase 4**: Frontend integration - uses Phase 3 components
5. **Phase 5**: Testing and refinement - validates entire system

This approach allows testing at each layer before moving to the next, ensuring solid foundations before adding complexity.

## Next Steps

1. Review and approve this plan
2. Begin with Phase 1, Task 1.1 (User and Identity Models)
3. Implement each task with accompanying tests
4. Run tests after each task to ensure functionality
5. Proceed to next task only when current task passes tests

## Notes on Future Extensibility

This architecture supports future enhancements:
- **Additional OAuth providers**: Easy to add more providers (Microsoft, Facebook, etc.) using the same pattern
- **Database storage**: Replace file system repositories with database
- **Distributed sessions**: Replace cookie sessions with Redis
- **Multi-factor authentication**: Add MFA service layer
- **Role-based access control**: Add roles/permissions to user model
- **API authentication**: Add JWT token generation for API access
