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
- Test configuration and setup
- Best practices for maintainable tests

### [Firebase Migration Plan](FIREBASE_MIGRATION_PLAN.md)
Plan for splitting the website into a separate `notes-webpage` repo hosted on
Firebase Hosting, with Firebase Auth replacing the current OAuth/cookie-session
system:
- Target Go JSON API surface and Firebase ID token verification approach
- File-by-file disposition of the current auth/template code
- Test migration strategy
- Sequenced implementation steps (not yet executed)

### [Storage Abuse Prevention](STORAGE_ABUSE_PREVENTION.md)
Analysis of how a public deployment could be abused to dump unbounded data
into the system, written against the post-migration (database + Firebase Auth)
architecture:
- Attack surface analysis: oversized notes, note flooding, update-churn
  amplification, and identity multiplication
- Why per-identity quotas are worthless without a scarce identity, and the
  recommendation on anonymous/guest write access
- Layered defenses (L1-L11): body caps, field validation, per-identity quotas,
  rate limiting, platform resource limits, timeouts, Firebase-side identity
  controls, and a global ceiling with kill switch
- A complete inventory of all 45 levers, each with a concrete aggressive value
- Aggressive TTL design and datastore-specific tradeoffs (Firestore vs Cloud SQL)

### [No Anonymous Server Access](NO_ANONYMOUS_SERVER_ACCESS.md)
How anonymous usage is eliminated entirely from the server's perspective, as
the counterpart to notes-webpage storing guest notes client-side:
- The invariant: no code path lets a caller without a federated identity write
  to persistent storage
- Enforcement via the token's `sign_in_provider` claim (allowlist, not denylist)
- What gets deleted, and which abuse levers this eliminates rather than bounds
- The one path where guest-originated data legitimately reaches the server
  (sign-in migration) and why it must be quota-checked

### [Cloud Run Deployment Plan](CLOUD_RUN_DEPLOYMENT_PLAN.md)
Plan for standing up the API on Google Cloud Run:
- Why the API must run with public ingress, and how Firebase ID token
  verification is the actual access boundary instead
- GCP's ingress vs. IAM invoker model and Workload Identity Federation
- Infrastructure task list, all 9 tasks implemented - split between
  one-time `gcloud` CI/CD setup and Terraform-managed application infra
  ([`infra/`](../infra))
- Known blocker: local-disk note storage is incompatible with Cloud Run's
  ephemeral filesystem

### [CI/CD Bootstrap](CLOUD_RUN_CICD_BOOTSTRAP.md)
One-time, manually-applied `gcloud` setup for the CI/CD identity itself -
the deploy service account, its IAM roles, Workload Identity Federation,
and the Artifact Registry repo - kept separate from the Terraform-managed
application infrastructure.

### [GCP Resources Created](CLOUD_RUN_GCP_RESOURCES.md)
Inventory of every GCP resource actually created while implementing the
plan above, why each one exists, and a cost analysis:
- What's billable vs. free, and current actual spend (effectively $0)
- Current exposure (none - no public invoker binding yet)
- Standby-state rationale for the current configuration

## Documentation Standards

When adding new documentation:
1. Use clear, descriptive filenames
2. Include a table of contents for longer documents
3. Provide code examples where applicable
4. Link to related documentation
5. Keep documentation up-to-date with code changes
