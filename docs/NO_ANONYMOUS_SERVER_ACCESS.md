# No Anonymous Server Access

How anonymous usage is eliminated entirely from the server's perspective:
what changes in the Go API, what is deleted, and what invariants must hold.

This is the **server-side counterpart** to
[notes-webpage's local-first guest storage design](https://github.com/bscheibe/notes-webpage/blob/main/docs/LOCAL_FIRST_GUEST_STORAGE.md).
That document explains where guest data goes instead (the visitor's own
browser). This one explains why the server stops having a guest concept at
all.

Neither is implemented yet. Both describe the target state.

## Contents

- [The invariant](#the-invariant)
- [Why this is worth doing](#why-this-is-worth-doing)
- [What "anonymous" means in three different places](#what-anonymous-means-in-three-different-places)
- [Enforcement](#enforcement)
- [What gets deleted](#what-gets-deleted)
- [What this eliminates from the abuse analysis](#what-this-eliminates-from-the-abuse-analysis)
- [The one path where anonymous data reaches the server](#the-one-path-where-anonymous-data-reaches-the-server)
- [Failure modes and edge cases](#failure-modes-and-edge-cases)
- [Verification](#verification)
- [What this does not solve](#what-this-does-not-solve)

## The invariant

> **Every request that reaches a storage operation carries a verified Firebase
> ID token whose sign-in provider is a federated identity provider.**

No unauthenticated path. No anonymous path. No guest path. No optional-auth
path. A request either presents a token from Google or GitHub, or it never
reaches the repository layer.

Stated as a negative, which is the more useful form for review: **there is no
code path by which a caller without a federated identity causes a byte to be
written to persistent storage.**

## Why this is worth doing

[STORAGE_ABUSE_PREVENTION.md](STORAGE_ABUSE_PREVENTION.md) establishes that a
storage quota is only as strong as the cost of minting a fresh identity, and
that anonymous identities are free and unlimited. Its central finding:

> Anonymous access and meaningful storage quotas are fundamentally in tension.

That report proposed bounding the problem — per-identity quotas, a global
ceiling, aggressive TTL. Those are sound, but they are all *mitigations*: they
make abuse survivable rather than impossible, and each one is code that can
regress.

Eliminating anonymous server access is categorically different. It removes the
attack surface rather than bounding it. There is no anonymous quota to tune
because there is no anonymous storage. The strongest security control is the
one you don't have to keep correct.

This also **preserves the guest feature**, which the abuse report had proposed
sacrificing. Guests still get a fully working app — their data simply lives in
their browser. The tradeoff that report framed (guest access *or* abuse
safety) turned out to be a false choice.

## What "anonymous" means in three different places

These are easy to conflate, and conflating them is how this design gets
implemented incorrectly. They are independent.

| Layer | Does anonymous exist? | Notes |
|---|---|---|
| **Product / UX** | **Yes** | Guests use the app fully. This is a real, supported mode. |
| **Firebase Auth** | **Optionally** | Anonymous Auth may stay enabled; the UID is a client-side key only. |
| **notes-app server** | **No** | The subject of this document. Zero anonymous concept. |

The key point: a guest existing as a *product concept* and even as a *Firebase
identity* does not mean the server knows about them. Under this design the Go
service cannot distinguish "a guest is using the app right now" from "nobody
is using the app" — because a guest generates no server traffic on the notes
path at all.

That is not a monitoring gap to fix. It is the design working.

## Enforcement

### Primary: reject non-federated tokens at the middleware

The planned `internal/firebaseauth/` middleware
([FIREBASE_MIGRATION_PLAN.md](FIREBASE_MIGRATION_PLAN.md#firebase-token-verification-in-go))
already verifies the token and extracts the UID. It gains one additional
check: the token's sign-in provider must be a federated one.

Firebase ID tokens carry a `firebase.sign_in_provider` claim. For anonymous
sign-in its value is `anonymous`; for federated sign-in it is `google.com` or
`github.com`. The check belongs in the middleware, applied to the whole
`/api/*` tree — not in individual handlers, where a newly added route could
omit it.

**Allowlist, do not denylist.** Accept a known set of federated providers and
reject everything else. `provider != "anonymous"` is the wrong shape: it
silently permits any provider added later (`password`, `phone`, custom
tokens), each of which reintroduces a cheap-identity problem. An allowlist
fails closed; a denylist fails open.

Rejections return `403` rather than `401` — the token is valid, the identity
type is not permitted. Distinguishable status codes matter here: a client
seeing `401` should refresh its token, while `403` means refreshing will never
help.

### Secondary: disable Anonymous Auth in the Firebase console

If the [open question](https://github.com/bscheibe/notes-webpage/blob/main/docs/LOCAL_FIRST_GUEST_STORAGE.md#open-questions)
in the frontend design resolves toward not needing `linkWithCredential`, then
turning the Anonymous provider off entirely means no anonymous token can be
minted at all. This is lever 37 in the abuse report and is a checkbox rather
than code.

The two controls are complementary, not redundant, and the ordering matters:
**the middleware check is primary.** The console toggle can be flipped by
anyone with project access and is invisible in code review, so it must never
be the only thing standing between an anonymous caller and the database.

### What must not be used as enforcement

- **`isAnonymous` from the client SDK.** A UI affordance, trivially forged. It
  is fine for deciding whether to show a "Guest" badge and nothing else.
- **Any client-sent header, body field, or query parameter.** All
  attacker-controlled.
- **Absence of an `Authorization` header.** Means unauthenticated, which is
  already rejected — but a *present* anonymous token is the case this document
  exists for, and it looks entirely normal until the claim is inspected.

Enforcement derives solely from claims inside a cryptographically verified
token.

## What gets deleted

The Firebase migration plan already removes most of the guest machinery. This
design removes the remainder, and — importantly — removes the *reason* to
reintroduce any of it.

Already slated for deletion by
[FIREBASE_MIGRATION_PLAN.md](FIREBASE_MIGRATION_PLAN.md#file-by-file-disposition-notes-app):

- `internal/middleware/auth_middleware.go` — including `OptionalAuth`
  ([auth_middleware.go:85](../internal/middleware/auth_middleware.go:85)), the
  function that mints a fresh UUID namespace per cookie-less request
  ([:99](../internal/middleware/auth_middleware.go:99)). This is the current
  anonymous-storage entry point.
- `internal/models/user.go` — `GuestSession`, `Identity`, `Provider`
  (including `ProviderGuest`).
- `internal/auth/` — the whole package.
- `internal/handlers/auth_handler.go` — including `HandleGuestLogin` and the
  `/auth/guest` route.

Additionally removed by this design:

- `Auth.GuestSessionDuration` config and `GetGuestSessionDuration()`
  ([config.go:150](../internal/config/config.go:150)), plus
  `guest_session_duration` from all three `config.*.yaml` files. Worth noting
  these are already **dead**: guest expiry is defined but never consulted on
  the note path, so it enforces nothing today.
- Any planned anonymous quota tier, anonymous TTL, or anonymous rate-limit
  bucket from [STORAGE_ABUSE_PREVENTION.md](STORAGE_ABUSE_PREVENTION.md).
  These become unreachable configuration.

**No new guest concept is introduced anywhere.** The frontend's guest mode
needs no server support: no registration, no session, no identifier, no
metrics dimension, no storage.

## What this eliminates from the abuse analysis

Against the [45-lever inventory](STORAGE_ABUSE_PREVENTION.md#complete-lever-inventory):

| Levers | Status |
|---|---|
| **7, 8** — anonymous note count / byte quotas | **Gone.** No anonymous storage to quota. |
| **9, 10** — anonymous rate limits | **Gone** on the notes path. |
| **14** — 24h anonymous TTL | **Gone.** Browser eviction replaces it, at no cost to us. |
| **37** — disable Anonymous Auth | Now optional rather than load-bearing. |
| **38** — App Check enforcing | **Downgraded.** Was required to make anonymous writes defensible; now a general hardening measure. |
| **39** — purge stale anonymous accounts | Only relevant if anonymous auth stays enabled; no notes are attached either way. |
| **41** — session cookie duration | **Gone** with cookie sessions. |

Most importantly, **A4 (identity multiplication)** — rated Critical, and the
vector that undermined every other control — is eliminated rather than
mitigated. Minting a million anonymous UIDs grants a million times zero server
storage.

The authenticated-tier levers all remain necessary and unchanged: body caps
(1–6), per-identity quotas (7–8 for federated identities), rate limiting,
global ceiling, TTL, timeouts, platform limits. **This design narrows the
problem; it does not solve storage abuse.** A determined attacker with a real
Google account is still in scope, and every control aimed at that case still
matters.

## The one path where anonymous data reaches the server

There is exactly one, and it must be designed deliberately rather than
discovered later.

**Sign-in migration.** When a guest signs in, their locally stored notes are
uploaded to their new authenticated account. Those bytes originated in an
anonymous context.

This does not violate the invariant — the import requests carry a federated
token, and the notes are stored against a federated UID. But it does mean:

- **The authenticated quota must apply to imports.** Otherwise the flow is a
  bypass: accumulate unlimited notes locally (deliberately unbounded — it's
  the user's own disk), sign in once, dump everything. Import must go through
  the same quota-checked write path as any other note.
- **Imported content is untrusted input.** It was under user control in
  `localStorage` and must be validated exactly like any request body: size
  caps, field limits, UTF-8 validity. No shortcut path that skips validation
  because "it came from our own frontend."
- **Prefer reusing the normal write endpoint** over a dedicated bulk-import
  route. A bulk route is a second write path that must independently
  re-implement every limit — and second paths are where limits get forgotten.
  If throughput demands batching later, the batch handler must call the same
  validated write logic per note, not a separate one.

## Failure modes and edge cases

- **Anonymous token presented directly to the API.** The expected attack —
  Anonymous Auth is enabled for the frontend, so anyone can mint a token and
  call the API with it. The middleware check is precisely what stops this. It
  is the single most important test case in the suite.
- **`linkWithCredential` upgrade.** After a guest links to Google, the UID
  stays the same but `sign_in_provider` becomes `google.com`. Tokens minted
  before the link still carry `anonymous` until refreshed. So a valid-looking
  token from a now-upgraded user may still be rejected until the client
  refreshes. Clients should force-refresh after linking; the server should not
  special-case this.
- **Provider added later.** Enabling `password` or `phone` sign-in in the
  Firebase console would grant server storage to a cheaper identity class
  without any code change. The allowlist means such a provider is rejected by
  default until deliberately added — which is the desired failure direction,
  but it means enabling a provider requires a matching code change and should
  be documented as such.
- **Custom tokens.** `sign_in_provider` is `custom`. Not used today; the
  allowlist rejects it. If custom tokens are ever introduced, this decision
  needs revisiting explicitly.
- **Token replay after account deletion.** Firebase ID tokens remain valid for
  up to an hour after issuance. A deleted user's token can still verify during
  that window. Relevant to storage only in that a deleted account could write
  briefly; acceptable for a demo, worth knowing.

## Verification

Tests that must exist, since this invariant is exactly the kind that silently
regresses:

- **Anonymous token → `403` on every `/api/*` route**, table-driven across the
  full route list so a newly added route without the check fails the suite.
  This is the core test.
- **Federated token (`google.com`, `github.com`) → allowed.**
- **Unknown/unexpected provider → `403`** (allowlist behavior, not denylist).
- **No token → `401`**, distinct from the `403` above.
- **No route reachable without the middleware.** Assert the router's mounted
  middleware chain, or that removing the check breaks tests — a route
  accidentally registered outside the authenticated group is the realistic
  regression.
- **Import path enforces quota** — a bulk import exceeding the authenticated
  quota is partially accepted up to the limit, not wholly accepted.
- **Grep-level check in review:** no reintroduction of `OptionalAuth`,
  `GuestSession`, `ProviderGuest`, or `guest_session_duration`.

A useful review heuristic: **if a code change makes the server aware that
guests exist, it is probably wrong.**

## What this does not solve

Stated plainly, so this document is not mistaken for a complete answer:

- **Storage abuse by authenticated users.** Fully in scope and unaddressed
  here. Every authenticated-tier lever in
  [STORAGE_ABUSE_PREVENTION.md](STORAGE_ABUSE_PREVENTION.md) still applies.
- **The cost of federated identities is not infinite.** Google and GitHub
  accounts are cheap in bulk to a motivated attacker. This raises the cost
  substantially; it does not make it prohibitive.
- **Compute abuse.** Unauthenticated requests still consume CPU to be rejected.
  Bounded by the existing Cloud Run caps
  ([cloud_run.tf](../infra/cloud_run.tf)), not by this design.
- **Content moderation**, out of scope in the abuse report and still out of
  scope here.

## Related

- [Storage Abuse Prevention](STORAGE_ABUSE_PREVENTION.md) — the full threat
  model and lever inventory this narrows
- [Firebase Migration Plan](FIREBASE_MIGRATION_PLAN.md) — target API shape and
  token verification approach
- [notes-webpage: Local-First Guest Storage](https://github.com/bscheibe/notes-webpage/blob/main/docs/LOCAL_FIRST_GUEST_STORAGE.md)
  — where guest data lives instead
