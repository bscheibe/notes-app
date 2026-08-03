# Notes App Documentation

This directory contains detailed documentation for the Notes App project.

## Available Documentation

### [Authentication Architecture](AUTHENTICATION_ARCHITECTURE.md)
Comprehensive overview of the authentication system design, including:
- OAuth integration with Google and GitHub
- Session management and security
- Guest session support
- User model and data structures
- Security considerations and best practices

### [Authentication Implementation Plan](AUTHENTICATION_IMPLEMENTATION_PLAN.md)
Detailed implementation plan for authentication features:
- OAuth provider setup and configuration
- User registration and management
- Session handling and middleware
- Guest session implementation
- Testing strategies for authentication

### [Integration Testing](INTEGRATION_TESTING.md)
Complete guide to testing strategies and implementation:
- Go unit and integration testing with Testify
- E2E testing with Playwright
- Page Object Model pattern
- Test configuration and setup
- Best practices for maintainable tests
- CI/CD integration examples

### [Firebase Migration Plan](FIREBASE_MIGRATION_PLAN.md)
Plan for splitting the website into a separate `notes-webpage` repo hosted on
Firebase Hosting, with Firebase Auth replacing the current OAuth/cookie-session
system:
- Target Go JSON API surface and Firebase ID token verification approach
- File-by-file disposition of the current auth/template code
- Test migration strategy
- Sequenced implementation steps (not yet executed)

### [Cloud Run Deployment Plan](CLOUD_RUN_DEPLOYMENT_PLAN.md)
Plan for standing up the API on Google Cloud Run:
- Why the API must run with public ingress, and how Firebase ID token
  verification is the actual access boundary instead
- GCP's ingress vs. IAM invoker model and Workload Identity Federation
- Infrastructure task list (Artifact Registry, service accounts, WIF, deploy
  workflow)
- Known blocker: local-disk note storage is incompatible with Cloud Run's
  ephemeral filesystem

## Documentation Standards

When adding new documentation:
1. Use clear, descriptive filenames
2. Include a table of contents for longer documents
3. Provide code examples where applicable
4. Link to related documentation
5. Keep documentation up-to-date with code changes
