# Integration Testing Documentation

## Overview

This document describes the integration testing approach for the Notes App, including framework choices, stdlib support, and implementation details.

## Go Testing Landscape

### Standard Library Support

Go has excellent built-in testing support in the `testing` package:

```go
package mypackage

import "testing"

func TestMyFunction(t *testing.T) {
    // Test implementation
}
```

**Stdlib Features:**
- **Testing Package**: Core testing framework with assertions
- **HTTP Testing**: `net/http/httptest` for HTTP server testing
- **IO Testing**: Built-in support for testing I/O operations
- **Benchmarking**: Built-in benchmarking support
- **Subtests**: Hierarchical test organization
- **Table-Driven Tests**: Idiomatic Go testing pattern

### Idiomatic Go Testing Frameworks

#### 1. **Testify** (Most Popular)
```go
import "github.com/stretchr/testify/assert"
import "github.com/stretchr/testify/require"
```

**Benefits:**
- Rich assertion library
- Mock generation
- Test suites
- Better error messages

**Why We Use It:**
- Industry standard for Go testing
- Comprehensive assertions
- Excellent documentation
- Minimal overhead

#### 2. **GoMock** (For Interfaces)
```go
import "github.com/golang/mock/gomock"
```

**Benefits:**
- Generated mocks from interfaces
- Type-safe mocking
- Built-in expectation verification

**Why We Didn't Use It:**
- Additional code generation step
- Overkill for our current needs
- Testify's simpler mocking sufficient

#### 3. **HTTP Expect** (For API Testing)
```go
import "github.com/gavv/httpexpect"
```

**Benefits:**
- Fluent HTTP testing API
- JSON assertion support
- Cookie and header handling

**Why We Didn't Use It:**
- We use stdlib `httptest` which is sufficient
- Keeps dependencies minimal
- Better integration with our handlers

## Our Integration Testing Approach

### Hierarchy of Tests

```
┌─────────────────────────────────────────────────────────────┐
│                  Integration Tests                          │
│  Full HTTP requests → Handlers → Services → Data Layer       │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                   Unit Tests                                 │
│  Individual functions and methods                           │
└─────────────────────────────────────────────────────────────┘
```

### Test Categories

#### 1. **Handler Integration Tests**
Location: `internal/handlers/*_integration_test.go`

**Purpose:** Test HTTP handlers with full request/response cycle

**What They Test:**
- HTTP request handling
- Response status codes
- Cookie management
- Session persistence
- CSRF protection
- Error handling
- Security headers

**Example:**
```go
func TestAuthHandlerIntegration_LoginFlow(t *testing.T) {
    // Setup real auth components
    // Make HTTP requests
    // Verify responses
    // Check side effects (session storage, etc.)
}
```

## Highest Leverage Integration Tests

### 1. **Complete Authentication Flow**

**Why High Leverage:**
- Tests critical security functionality
- Covers multiple components (handlers, services, storage)
- Validates user-facing behavior
- Catches integration issues early

**Test Coverage:**
- Guest login flow
- OAuth initiation
- Session management
- Logout flow
- Cookie security

### 2. **Protected Route Access**

**Why High Leverage:**
- Tests security middleware
- Validates authorization logic
- Covers redirection behavior
- Tests session validation

**Test Coverage:**
- Unauthenticated access attempts
- Authenticated access
- Guest vs user distinction
- Cookie-based authentication

### 3. **OAuth CSRF Protection**

**Why High Leverage:**
- Tests critical security feature
- Validates state parameter handling
- Tests session-based protection
- Prevents security vulnerabilities

**Test Coverage:**
- State generation
- State validation
- Session storage
- CSRF attack simulation

### 4. **Session Persistence and Expiry**

**Why High Leverage:**
- Tests core data persistence
- Validates time-based functionality
- Tests cleanup mechanisms
- Covers storage layer integration

**Test Coverage:**
- Session creation
- Session retrieval
- Session validation
- Expiry handling
- Multiple concurrent sessions

### 5. **Server Middleware Chain**

**Why High Leverage:**
- Tests request processing pipeline
- Validates middleware order
- Tests cross-cutting concerns
- Covers error handling

**Test Coverage:**
- Request ID generation
- Logging middleware
- Recovery middleware
- Timeout handling
- Security headers

## Implementation Patterns

### 1. **Test Server Pattern**

```go
// Create test server
testServer := httptest.NewServer(server.router)
defer testServer.Close()

// Make real HTTP requests
resp, err := http.Get(testServer.URL + "/login")
```

**Benefits:**
- Real HTTP client
- Full request/response cycle
- Tests routing and middleware
- Easy to test with cookies

### 2. **Temporary Directory Pattern**

```go
tempDir := t.TempDir()

// Use tempDir for file storage
userRepo, _ := auth.NewFileSystemUserRepository(filepath.Join(tempDir, "users"))
```

**Benefits:**
- Automatic cleanup
- Isolated test environments
- No side effects between tests
- Safe for concurrent tests

### 3. **Table-Driven Test Pattern**

```go
tests := []struct {
    name     string
    setup    func()
    expected int
}{
    {"test1", setup1, 200},
    {"test2", setup2, 404},
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        tt.setup()
        // Test implementation
    })
}
```

**Benefits:**
- DRY test code
- Easy to add test cases
- Clear test intent
- Better test organization

### 4. **Cookie Management Pattern**

```go
// Extract cookie from response
cookies := resp.Cookies()
var sessionCookie *http.Cookie
for _, cookie := range cookies {
    if cookie.Name == "test_session" {
        sessionCookie = cookie
        break
    }
}

// Add cookie to subsequent request
req.AddCookie(sessionCookie)
```

**Benefits:**
- Tests session persistence
- Validates cookie security flags
- Tests multi-request flows
- Simulates real browser behavior

## Test Coverage Strategy

### Critical Paths (Highest Priority)
1. ✅ Authentication flows (login, logout, OAuth)
2. ✅ Protected route access
3. ✅ Session management
4. ✅ CSRF protection
5. ✅ Error handling

### Important Paths (Medium Priority)
1. ✅ Health endpoints
2. ✅ Middleware chain
3. ✅ Concurrent request handling
4. ✅ Configuration validation

### Nice to Have (Lower Priority)
1. Performance testing
2. Load testing
3. Integration with external services

## Running Integration Tests

### Run All Integration Tests
```bash
go test ./... -run Integration
```

### Run Specific Integration Test
```bash
go test ./internal/handlers -run TestAuthHandlerIntegration
```

### Run with Verbose Output
```bash
go test ./... -v -run Integration
```

### Run with Coverage
```bash
go test ./... -cover -run Integration
```

## Continuous Integration

Integration tests run as part of the standard `go test ./... -race -shuffle=on`
step in [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) - there's no
separate CI job for them.

## Go Integration Testing Best Practices

### 1. **Test Isolation**
- Each test should be independent
- Use temporary directories for file operations
- Clean up resources in defer statements
- Avoid shared state between tests

### 2. **Realistic Scenarios**
- Test actual user flows, not just individual functions
- Use real HTTP clients and servers
- Test with realistic data
- Include edge cases and error conditions

### 3. **Clear Test Names**
```go
// Good
func TestAuthHandlerIntegration_GuestLoginFlow(t *testing.T)

// Bad
func TestAuthHandlerIntegration(t *testing.T)
```

### 4. **Use Subtests for Organization**
```go
func TestAuthHandlerIntegration(t *testing.T) {
    t.Run("guest login", func(t *testing.T) { ... })
    t.Run("OAuth flow", func(t *testing.T) { ... })
    t.Run("logout", func(t *testing.T) { ... })
}
```

### 5. **Test Failures Should Be Informative**
```go
// Good
assert.Equal(t, http.StatusOK, resp.StatusCode, 
    "Expected successful response for guest login")

// Bad
assert.Equal(t, http.StatusOK, resp.StatusCode)
```

## Limitations and Future Improvements

### Current Limitations
1. **No External Service Mocking**: OAuth providers not mocked
2. **No Database Testing**: File-based storage only
3. **No Performance Testing**: No load or stress testing

### Future Improvements
1. **Add OAuth Mocking**: Mock OAuth provider responses
2. **Add Database Tests**: Test with real database in CI
3. **Add Performance Tests**: Benchmark critical paths
4. **Add Chaos Testing**: Test failure scenarios

## Comparison with Alternatives

### Why Not Use Full E2E Framework?

Browser-driven end-to-end testing lives with the frontend, in
[notes-webpage](https://github.com/bscheibe/notes-webpage) - this repo is a
JSON API with no UI to drive.

**Postman/Newman:**
- Pros: API testing, collection management
- Cons: Separate from codebase, harder to maintain
- Decision: Go native tests better for our workflow

### Why Not Use Test Containers?

**Testcontainers:**
- Pros: Real dependencies (databases, services)
- Cons: Complex setup, slower tests
- Decision: File-based storage sufficient for now

## Conclusion

Our integration testing approach leverages Go's excellent stdlib support along with Testify for assertions. We focus on testing the highest-leverage functionality - authentication flows, security features, and core user interactions. The tests are designed to be fast, reliable, and maintainable while providing confidence that the application works correctly as an integrated system.

The chosen approach balances thoroughness with practicality, ensuring critical paths are well-tested without over-engineering the test suite. As the application grows, we can expand coverage to include performance testing and external service mocking.
