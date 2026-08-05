# Storage Abuse Prevention

Analysis of how a malicious actor could dump unbounded data into `notes-app`
once it is opened to the public, and what to do about it before that happens.

Written against the **post-migration** architecture — database-backed storage
plus Firebase Auth — since that is what will actually be exposed. Where the
current code is cited, it is to show which abuse paths are inherited by the
migration rather than fixed by it.

## Contents

- [Scope and threat model](#scope-and-threat-model)
- [Current state: what exists today](#current-state-what-exists-today)
- [Attack surface analysis](#attack-surface-analysis)
- [The identity problem: why quotas need a scarce key](#the-identity-problem-why-quotas-need-a-scarce-key)
- [Defense layers](#defense-layers)
- [Complete lever inventory](#complete-lever-inventory)
- [Recommended quota values](#recommended-quota-values)
- [TTL / retention design](#ttl--retention-design)
- [Guest access: recommendation](#guest-access-recommendation)
- [Datastore-specific notes](#datastore-specific-notes)
- [Observability and alerting](#observability-and-alerting)
- [Implementation plan](#implementation-plan)
- [Explicitly out of scope](#explicitly-out-of-scope)

## Scope and threat model

This document covers **storage abuse**: causing the system to persist more
data, or more expensive data, than intended. It is scoped to that. Adjacent
concerns (auth bypass, XSS, data exfiltration, note-content abuse as a CDN for
illegal material) are noted where they intersect but are not the subject.

### Assets at risk

| Asset | Failure mode |
|---|---|
| Database storage | Unbounded rows/bytes → cost, eventually a full or throttled instance |
| Database write throughput | Sustained writes → cost, contention, degraded service for real users |
| Cloud Run compute | Request volume → instance-hours, capped at 3 instances today |
| Egress bandwidth | Large notes read back repeatedly |
| The demo itself | A full or throttled datastore takes the app down |

### Adversary profiles

1. **Casual script kiddie.** A `for` loop with `curl` against a public
   endpoint. No auth automation. Highest likelihood by far, lowest
   sophistication. Defeated by per-identity quotas plus rate limiting.
2. **Motivated single actor.** Automates Firebase Anonymous Auth to mint
   identities, distributes writes across them. Defeats naive per-user quotas.
   This is the profile that actually drives the design below.
3. **Distributed / botnet.** Many IPs, many identities. Not realistically
   defensible at the app layer for a demo app; the answer is a hard global
   ceiling and a kill switch, accepting denial-of-service for legitimate
   users over an unbounded bill.

**Design assumption:** this is a demonstration application. The correct
posture is *cost containment and availability of the demo*, not
five-nines durability of user data. That assumption justifies aggressive
TTLs and low quotas that would be unacceptable in a real product, and it is
what makes profile 3 tolerable to handle bluntly.

## Current state: what exists today

The following is what the code does **now**, verified in the repo at the time
of writing. All of it is pre-migration, but the gaps are inherited unless
explicitly addressed.

### There are no limits of any kind

There is no request body size limit, no note count limit, no note size limit,
no per-user quota, no rate limiting, and no retention policy anywhere in the
codebase. The write path is:

`HandleSaveNote` ([internal/handlers/note_handler.go:177](../internal/handlers/note_handler.go:177))
→ `CreateNote`/`UpdateNote` ([internal/service/note_service.go:52](../internal/service/note_service.go:52))
→ `Save` ([internal/repository/note_repository.go:96](../internal/repository/note_repository.go:96))

The only validation in that entire path is in `validateRequest`
([internal/service/note_service.go:98](../internal/service/note_service.go:98)):

```go
func (s *NoteService) validateRequest(req *models.CreateNoteRequest) error {
	if req.Title == "" {
		return ErrTitleRequired
	}
	if req.Content == "" {
		return ErrContentRequired
	}
	return nil
}
```

Title and content must be non-empty. That is the complete set of rules. A
single-byte title and a 500 MB content body both pass.

### The request body is read unbounded

`HandleSaveNote` calls `r.ParseForm()`
([internal/handlers/note_handler.go:187](../internal/handlers/note_handler.go:187))
with no `http.MaxBytesReader` wrapping `r.Body`. For a URL-encoded form,
`ParseForm` reads the entire body into memory before the handler sees a
single field. Go's `http.Server` has no default body size cap. The only
current bounds are Cloud Run's own request limit (32 MiB for non-streamed
requests) and the 15-second `ReadTimeout`
([internal/server/server.go:173](../internal/server/server.go:173)) — both
incidental, neither chosen for this purpose.

This is a memory-exhaustion vector *before* it is a storage vector: with
`max_instance_request_concurrency = 10`
([infra/cloud_run.tf:23](../infra/cloud_run.tf:23)), ten concurrent 32 MiB
uploads means ~320 MiB of form buffers on one instance.

### Note updates can silently multiply files

`generateFilename`
([internal/repository/note_repository.go:150](../internal/repository/note_repository.go:150))
is the most important detail in the current implementation:

```go
func (r *NoteRepository) generateFilename(title, originalFilename string) string {
	sanitizedTitle := sanitizeFilename(title)

	if originalFilename != "" {
		expectedPrefix := sanitizedTitle + "-"
		if strings.HasPrefix(originalFilename, expectedPrefix) {
			// Title same, update existing file
			return originalFilename
		}
	}

	// New note or title changed, create new file with timestamp
	timestamp := time.Now().Format("2006-01-02-15-04-05")
	return fmt.Sprintf("%s-%s.md", sanitizedTitle, timestamp)
}
```

An "update" that changes the title does not update — it **creates a new
record and orphans the old one**. There is no delete of the previous file.
So `PUT /api/notes/{filename}` with a rotating title is an unbounded
*create* primitive wearing an update's clothing. An attacker doesn't even
need to call the create endpoint.

Worse, the timestamp has one-second granularity. Two updates within the same
second with the same changed title collide onto one filename and silently
overwrite each other — so the function is simultaneously a data-loss bug and
an unbounded-growth path. The migration to a database with a real primary key
should delete this function outright rather than port it.

> **Note beyond storage scope:** `sanitizeFilename`
> ([internal/repository/note_repository.go:183](../internal/repository/note_repository.go:183))
> filters the *title* to alphanumerics on write, but `Get` and `Delete`
> ([note_repository.go:69](../internal/repository/note_repository.go:69),
> [:129](../internal/repository/note_repository.go:129)) `filepath.Join` the
> caller-supplied `filename` with no sanitization at all, so a `filename` of
> `../../etc/passwd` traverses out of the user's directory. The database
> migration eliminates this by construction, which is one more reason to do
> the migration before going public. Flagged here because it was found during
> this review; it is a path-traversal bug, not a storage-abuse one.

### Guest sessions are an unlimited namespace factory

`OptionalAuth`
([internal/middleware/auth_middleware.go:93](../internal/middleware/auth_middleware.go:93))
mints a fresh UUID for any request arriving without a valid session cookie:

```go
sessionID, ok := session.Values["session_id"].(string)
sessionCreated := false
if !ok || sessionID == "" {
	sessionID = uuid.New().String()
	session.Values["session_id"] = sessionID
	...
}
```

Discarding the cookie and re-requesting yields a brand-new isolated storage
namespace, free, instantly, with no proof-of-work and no cost to the caller.
Any per-user quota is trivially bypassed by not sending a cookie. **This is
the single most important finding in this document**, and the reason
[guest access needs a decision](#guest-access-recommendation).

Note also that `GuestSession.IsValid()`
([internal/models/user.go](../internal/models/user.go)) and
`GetGuestSessionDuration()`
([internal/config/config.go:150](../internal/config/config.go:150)) exist but
are not consulted anywhere in the note read/write path — guest expiry is
defined but not enforced against stored data.

### Cost caps exist, but only for compute

`infra/cloud_run.tf` sets `max_instance_count = 3`,
`max_instance_request_concurrency = 10`, and `timeout = "30s"`
([infra/cloud_run.tf:11-24](../infra/cloud_run.tf:11)). This is real and
useful — it bounds the *compute* bill and provides natural backpressure
(excess requests get `429`s rather than scaling out). Credit where due: this
was a deliberate choice, documented as task 9 of the deployment plan.

But it does nothing for storage. Three instances at concurrency 10 can still
write to the database continuously, forever. Compute cost is capped; storage
cost is not. Storage is monotonic — it accumulates and persists after the
attack stops, whereas compute cost ends when the traffic does.

Note the `max_instance_count = 3` cap also has a defensive downside worth
naming: it means a modest flood of abusive traffic is enough to deny service
to legitimate users, because there is no priority separation between them.
That tradeoff is acceptable for a demo, but it means rate limiting should be
seen as *protecting the demo's availability*, not just the bill.

### What the migration fixes on its own — and what it does not

**Fixes:** ephemeral-disk data loss; path traversal via `filepath.Join`;
filename-collision overwrites; the per-second filename granularity problem.

**Does not fix — inherited unchanged:** every quota gap above. Critically,
the Firebase migration *preserves the guest abuse shape* rather than
resolving it. From
[FIREBASE_MIGRATION_PLAN.md:60](FIREBASE_MIGRATION_PLAN.md:60):

> **Guests are not a special case in Go.** Firebase Anonymous Auth issues a
> real, stable Firebase UID for guests just like it does for signed-in users.

That is correct and good for *code simplicity* — the Go side stops needing an
`OptionalAuth` branch. But from an abuse standpoint the property that matters
is unchanged: **anonymous UIDs are free and unlimited to mint.** Firebase
Anonymous Auth issues a UID to anyone who asks, with no email, no OAuth
provider, and no human in the loop. The migration converts "throw away the
cookie" into "call `signInAnonymously()` again" — a marginally higher bar
that any script clears in one line. The abuse economics are identical.

## Attack surface analysis

Each vector below is rated for **severity** (cost/impact if exploited) and
**effort** (how hard for the attacker).

### A1 — Large single note

Severity: **High** · Effort: **Trivial**

`POST /api/notes` with a multi-megabyte `content` field. Nothing in
`validateRequest` bounds it; nothing bounds the body read. A handful of
requests puts hundreds of megabytes into the database. In a row-oriented
store this also destroys read performance for the owning user, since `GET
/api/notes` in the current shape would pull content along with metadata.

**Mitigations:** [L2](#l2--request-body-cap) body cap,
[L3](#l3--field-level-validation) field validation.

### A2 — Many small notes

Severity: **High** · Effort: **Trivial**

A loop issuing `POST /api/notes` with a 1-byte body. Each row is small, but
per-row overhead (primary key, indexes, timestamps, per-document metadata)
dominates, and the row *count* is what degrades list queries and drives
per-operation billing in Firestore-style stores. At Cloud Run's capped
throughput this is still on the order of tens of thousands of rows per
minute.

**Mitigations:** [L4](#l4--per-identity-quotas) note-count quota,
[L5](#l5--rate-limiting) rate limiting.

### A3 — Update-churn amplification

Severity: **High** · Effort: **Trivial** · *Specific to the current code*

As described above: `PUT` with a rotating title creates a new record each
time and orphans the old, so an attacker consumes storage through the update
endpoint while never touching create. Any quota implemented as "count of
create calls" misses this entirely.

**Mitigations:** delete `generateFilename` during the migration; enforce
quotas on **stored state** (rows/bytes owned), not on operation counts.

This vector generalizes into a rule worth stating plainly, because it is the
most common way quota systems fail in practice:

> Quota checks must be against what the user currently *owns*, computed at
> write time — never against a running tally of operations performed.

An operation counter can always be defeated by finding an operation the
counter doesn't count. A state check cannot.

### A4 — Identity multiplication

Severity: **Critical** · Effort: **Low**

The profile-2 attack, and the one that breaks the others' mitigations.
Pre-migration: drop the cookie, get a new UUID namespace
([auth_middleware.go:99](../internal/middleware/auth_middleware.go:99)).
Post-migration: call Firebase `signInAnonymously()` repeatedly.

The multiplier is unbounded. If the per-user quota is 100 notes and an
attacker mints 10,000 anonymous UIDs, the effective quota is 1,000,000 notes.
**Per-identity quotas are worthless without a bound on identity creation.**

**Mitigations:** [L1](#l1--identity-scarcity) identity scarcity — the
foundational layer; [L6](#l6--global-ceiling-and-kill-switch) global ceiling.

### A5 — Compression bombs and encoding tricks

Severity: **Medium** · Effort: **Low**

If the API accepts `Content-Encoding: gzip`, a small compressed body inflates
to a large one *after* the body-size check if the check is applied to the
compressed stream. A few kilobytes on the wire becomes hundreds of megabytes
in memory and storage.

**Mitigation:** apply `http.MaxBytesReader` to the **decompressed** stream,
or decline compressed request bodies entirely. For a JSON API of this size,
declining them is simpler and costs nothing — notes are small by policy
anyway. See [L2](#l2--request-body-cap).

Related: multi-byte UTF-8. A limit expressed in "characters" via
`len([]rune(s))` allows up to 4× the bytes of the same limit in
`len(s)`. **Validate in bytes**, since bytes are what gets billed.

### A6 — Storage via metadata rather than content

Severity: **Low** · Effort: **Trivial**

If `content` is capped but `title` is not, the title becomes the payload.
Every user-controlled field needs its own bound, and there should be a cap on
the total serialized record — not just a sum of per-field caps, which is
easier to reason about and harder to get wrong as fields are added.

**Mitigation:** [L3](#l3--field-level-validation).

### A7 — Soft-delete and TTL evasion

Severity: **Medium** · Effort: **Low**

Two failure modes worth designing against up front:

- If deletes are soft (tombstone rows) and quotas count only live rows, an
  attacker cycles create → delete → create indefinitely, growing the table
  while never exceeding quota.
- If TTL is measured from *last modified* rather than *creation*, an attacker
  keeps data alive forever by touching each note periodically. A trivial
  cron-like loop defeats the entire retention policy.

**Mitigations:** count tombstones against quota until purged, or hard-delete;
anchor TTL to creation time. See [TTL design](#ttl--retention-design).

### A8 — Read-side amplification

Severity: **Low** · Effort: **Trivial**

Not storage growth, but the same bill: repeatedly `GET`ing large notes drives
egress and per-read billing. Largely mitigated by capping note size in the
first place, plus the existing Cloud Run instance cap. Worth a pagination
limit on `GET /api/notes` so a list call can never return an unbounded result
set.

## The identity problem: why quotas need a scarce key

Every quota is a function of an identity key. The quota is only as strong as
the cost of obtaining a fresh key. This is worth stating explicitly because
it determines whether the rest of the design is meaningful or theater:

| Quota key | Cost to attacker of a new key | Verdict |
|---|---|---|
| Session cookie (today) | Zero — discard and re-request | Useless |
| Firebase anonymous UID | Near-zero — one API call | Useless alone |
| IP address | Low — proxies, mobile networks, cloud IPs | Weak, and punishes shared NATs |
| Firebase UID from Google/GitHub OAuth | Real — requires a provider account | **Strong** |
| Verified email | Moderate — disposable-mail services exist | Moderate |

The conclusion is uncomfortable but clean: **anonymous access and meaningful
storage quotas are fundamentally in tension.** No amount of per-user limit
tuning fixes an identity that costs nothing to mint. You can bound anonymous
abuse only by (a) removing anonymous write access, (b) giving anonymous users
a global shared pool rather than per-identity quotas, or (c) accepting a
capped worst case and building a kill switch.

Note that IP-based limiting is *not* a fix here, and is weaker in this
deployment than it looks: `chiMiddleware.RealIP` is deliberately not mounted
([internal/server/server.go:108](../internal/server/server.go:108)) because
it trusts `X-Forwarded-For` unconditionally. That reasoning is sound. But it
means that behind Cloud Run's proxy, `RemoteAddr` is not the client's
address, so per-client IP limiting requires carefully parsing
`X-Forwarded-For` — specifically, taking the **rightmost** entry appended by
the trusted proxy rather than the leftmost, which is client-controlled and
spoofable. Getting this backwards is the standard way IP rate limiting
becomes trivially bypassable.

## Defense layers

Ordered by the level at which they act. Each is independently valuable; the
lower-numbered ones matter most.

### L1 — Identity scarcity

The foundation. Options, in descending order of recommendation:

1. **Require a federated provider (Google/GitHub) for all writes.** Anonymous
   users may read a read-only demo dataset but cannot persist anything.
   Reduces the quota problem to "how many Google accounts can an attacker
   get," which is a real cost. **Recommended.**
2. **Allow anonymous writes against a single shared global pool** — e.g. all
   anonymous UIDs collectively get 500 notes, evicted oldest-first. Preserves
   the try-before-signup experience with a *hard* bound, and degrades to
   "anonymous users evict each other" under attack rather than to an
   unbounded bill. Good fallback if the frictionless demo matters.
3. **Anonymous writes with per-UID quotas.** Only acceptable with
   [L6](#l6--global-ceiling-and-kill-switch) as the actual backstop, because
   per-UID quotas alone do not bound the total.

Whichever is chosen, enforce it server-side from the verified token's claims.
Firebase ID tokens expose `firebase.sign_in_provider` (and the token's
`provider_id`); `anonymous` is a distinguishable value. Do not trust a
client-sent flag, and do not use `isAnonymous` from the JS SDK for
enforcement — that is a UI affordance, not a security control.

### L2 — Request body cap

Wrap every request body before parsing:

```go
const maxRequestBody = 64 * 1024 // 64 KiB

r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
```

`MaxBytesReader` is the right tool over a manual `io.LimitReader`: it makes
subsequent reads fail with a `*http.MaxBytesError` (so a `413` can be
returned instead of a confusing `400`) and it signals the server to close the
connection rather than keep draining an attacker's stream.

Mount it as middleware for the whole `/api` tree, not per handler, so a new
endpoint can't be added without it. Reject `Content-Encoding: gzip` on
request bodies ([A5](#a5--compression-bombs-and-encoding-tricks)) unless a
concrete need appears.

This must happen **before** `ParseForm`/`json.Decode`, since those read the
whole body. Once the migration lands, prefer `json.Decoder` with
`DisallowUnknownFields()` over `ParseForm` — the JSON API shape makes the
current form parsing obsolete anyway.

### L3 — Field-level validation

Extend `validateRequest`
([internal/service/note_service.go:98](../internal/service/note_service.go:98))
with **byte-length** bounds. This belongs in the service layer, not the
handler, so it holds regardless of transport:

```go
const (
	maxTitleBytes   = 200
	maxContentBytes = 32 * 1024 // 32 KiB
)

func (s *NoteService) validateRequest(req *models.CreateNoteRequest) error {
	if req.Title == "" {
		return ErrTitleRequired
	}
	if len(req.Title) > maxTitleBytes {
		return ErrTitleTooLong
	}
	if req.Content == "" {
		return ErrContentRequired
	}
	if len(req.Content) > maxContentBytes {
		return ErrContentTooLong
	}
	return nil
}
```

`len()` on a string is bytes, which is what to bound — see
[A5](#a5--compression-bombs-and-encoding-tricks). Also reject invalid UTF-8
(`utf8.ValidString`) so malformed input can't cause surprises downstream.

The new errors need cases in `getErrorMessage`
([internal/handlers/note_handler.go:273](../internal/handlers/note_handler.go:273))
and should map to `400`, distinct from the `413` that L2 produces.

### L4 — Per-identity quotas

Two quotas, both evaluated against **currently-stored state** per
[A3](#a3--update-churn-amplification):

- **Note count** per identity.
- **Total bytes** per identity — the one that actually bounds cost. A count
  limit alone permits `N × maxContentBytes`.

Enforce at write time, inside the same transaction as the insert, or the
check is racy: concurrent requests each read a below-limit count and each
proceed. With `max_instance_request_concurrency = 10` across up to 3
instances, that race is reachable in practice, not theoretical. Options:

- SQL: `SELECT ... FOR UPDATE` on a per-user quota row, or a `CHECK`-backed
  counter column updated in the same transaction.
- Firestore: a per-user counter document updated in the same transaction as
  the note write.

Maintain a denormalized per-user `note_count` / `total_bytes` rather than
computing `COUNT(*)`/`SUM(length(content))` per write — the aggregate query
becomes its own DoS vector once a user has many rows.

Return `429` (or `403` with a machine-readable code) with a clear message, so
the frontend can distinguish "you're at your limit" from a transient failure.

### L5 — Rate limiting

Quotas bound the steady state; rate limits bound the *slope*, which is what
determines whether an attack is noticed before it matters. Even with a
correct quota, an attacker who reaches it in two seconds across many
identities produces a spike no alert can react to.

Apply a token-bucket limiter on writes, keyed by verified UID, with a modest
burst (a real user saving a few notes quickly should never hit it). A
per-instance in-memory limiter (e.g. `golang.org/x/time/rate` behind an
LRU keyed by UID) is adequate here: with `max_instance_count = 3` the effective
global rate is at most 3× the per-instance rate, which is a known, bounded
overshoot. A shared store (Redis/Firestore) would make it exact but adds a
dependency and a per-request round trip that a demo app does not need.

Keying by UID rather than IP avoids the `X-Forwarded-For` parsing pitfall
described above. Keep an IP-keyed limiter only for unauthenticated endpoints,
if any remain.

### L6 — Global ceiling and kill switch

The backstop for [A4](#a4--identity-multiplication), and the only layer that
bounds profile 3. Everything above is per-identity and therefore
multipliable; this is not.

- A **global storage ceiling**: total rows / total bytes across all users.
  On breach, reject all writes with `503` and alert. The demo goes read-only
  rather than unboundedly expensive — the correct tradeoff for a demo.
- A **global write-rate ceiling**, likewise.
- A **manual kill switch**: a config flag or env var that puts the API in
  read-only mode without a redeploy, so a human can stop an in-progress
  attack in seconds.
- A **GCP budget alert** on the billing account. Already noted as unconfigured
  in [CLOUD_RUN_GCP_RESOURCES.md](CLOUD_RUN_GCP_RESOURCES.md#standby-state);
  it should be configured before going public. It does not *prevent* spend,
  but it is the last line of notification if every app-layer control fails.

### L7 — Retention / TTL

Covered separately in [TTL design](#ttl--retention-design). Distinct from the
layers above: those bound the rate and the peak, TTL bounds the **integral** —
it guarantees that any successful abuse is self-cleaning rather than
permanent. For a demo app this is unusually valuable, because it converts a
permanent storage cost into a temporary one.

### L8 — Platform resource limits

`infra/cloud_run.tf` sets scaling, concurrency, and request timeout, but
**declares no `resources` block** — so the container runs at Cloud Run's
defaults (512 MiB memory, 1 CPU). That matters for
[A5](#a5--compression-bombs-and-encoding-tricks): with
`max_instance_request_concurrency = 10`, ten concurrent bodies share 512 MiB.
Under the current unbounded `ParseForm` that is an OOM long before it is a
storage problem; with [L2](#l2--request-body-cap)'s 64 KiB cap in place, ten
concurrent bodies are ~640 KiB and the risk disappears.

Set the limits explicitly rather than inheriting defaults, so the ceiling is a
decision rather than an accident:

```hcl
resources {
  limits = {
    cpu    = "1"
    memory = "512Mi"
  }
  cpu_idle = true  # don't bill CPU between requests
}
```

Aggressive posture: keep **512 MiB** — do not raise it. A memory limit is a
useful circuit breaker. If a body-size bug ever regresses, an instance that
OOMs and restarts is a far better failure mode than one that quietly absorbs
a 400 MiB upload and writes it to the database. Resist the reflex to raise
memory when you see OOM kills; investigate what is consuming it first.

Also worth setting `max_instance_count` per-revision rather than relying on
the current `3`, and adding a **Firestore/Cloud SQL-side quota** if the store
supports one, so the datastore itself refuses writes past a ceiling
independent of application logic.

### L9 — Timeouts

Long-running requests hold instance slots and let an attacker trickle a large
body in slowly. Current values, and a conflict worth fixing:

| Setting | Location | Value |
|---|---|---|
| `chiMiddleware.Timeout` | [server.go:112](../internal/server/server.go:112) | 60s |
| `http.Server.ReadTimeout` | [server.go:173](../internal/server/server.go:173) | 15s |
| `http.Server.WriteTimeout` | [server.go:174](../internal/server/server.go:174) | 15s |
| `http.Server.IdleTimeout` | [server.go:175](../internal/server/server.go:175) | 60s |
| Cloud Run `timeout` | [cloud_run.tf:24](../infra/cloud_run.tf:24) | 30s |

**The 60-second chi timeout can never fire** — `WriteTimeout` at 15s kills the
response first, and Cloud Run caps at 30s. It is dead configuration that
misleads anyone reading it as the effective limit. Either drop it or set it
below `WriteTimeout`.

Aggressive posture: `ReadTimeout` **5s**, `ReadHeaderTimeout` **2s**,
`WriteTimeout` **10s**, `IdleTimeout` **30s**, Cloud Run `timeout` **10s**,
chi timeout **8s** (or removed). Notes are small and the datastore is fast;
no legitimate request to this API needs seconds. A short `ReadTimeout` is
specifically what defeats slow-body attacks — it bounds the *duration* of a
body read where [L2](#l2--request-body-cap) bounds only its size, and the two
are independent. `ReadHeaderTimeout` is currently unset and should not be:
it is the specific defense against Slowloris-style header trickling.

### L10 — Firebase-side controls

Enforcement that lives in Firebase rather than in Go, and therefore acts
before a request ever reaches Cloud Run — the cheapest possible place to
reject abuse:

- **Disable Anonymous Auth outright** in the Firebase console if
  [L1](#l1--identity-scarcity) option 1 is taken. This is the single most
  effective control available and it is a checkbox, not code. It makes
  [A4](#a4--identity-multiplication) structurally impossible rather than
  merely bounded. If anonymous sign-in is disabled, no token with
  `sign_in_provider == "anonymous"` can be minted at all, so the Go-side check
  becomes defense in depth rather than the primary gate.
- **Firebase App Check** with reCAPTCHA Enterprise (web) attests that requests
  come from your actual frontend rather than a script. This is the one control
  that meaningfully raises the cost of [A4](#a4--identity-multiplication)
  *while keeping anonymous access*. If frictionless anonymous writes are
  judged essential, App Check is what makes that defensible — enable it in
  enforcing mode, not monitoring mode, and verify the token server-side.
- **Identity Platform quotas** — if upgraded from base Firebase Auth, sign-up
  rate limits per IP are configurable. Base Firebase Auth has an undocumented
  anonymous sign-in rate limit that should not be relied on as a control.
- **Blocking functions** (`beforeCreate`) can reject anonymous sign-ups
  entirely or apply custom logic at identity-creation time.
- **Periodically purge anonymous accounts.** Firebase does not do this
  automatically, and unused anonymous UIDs accumulate in the user table
  indefinitely. Delete anonymous accounts older than the anonymous TTL (24h) —
  this keeps the identity table bounded alongside the note table, and matching
  the two windows means an expired identity never has surviving notes.

### L11 — Session and cookie lifetime *(pre-migration only)*

Applies to the current cookie system, which the Firebase migration removes.
Included because it governs how fast guest namespaces recycle *today*, and
because the app may sit publicly reachable in its current form before the
migration lands.

`config.prod.yaml` sets `duration: "168h"` (7 days) and
`guest_session_duration: "24h"`. The store's `MaxAge`
([auth_handler.go:29](../internal/handlers/auth_handler.go:29)) derives from
the former. Cookie flags are already correct — `HttpOnly`, `SameSite=Lax`, and
`Secure` outside localhost.

Note the guest duration is **not enforced against stored notes** anywhere;
`GuestSession.IsValid()` is never consulted on the note path. So a shorter
guest session does not currently reclaim any storage — it only forces a new
namespace, which if anything *accelerates* [A4](#a4--identity-multiplication).
Do not shorten it as an abuse control; it would make things worse. Fix it by
enforcing expiry against data ([L7](#l7--retention--ttl)), not by tuning the
cookie.

Aggressive posture if going public pre-migration: session `24h` (not 168h),
and treat this as a stopgap — the real fix is the migration.

## Complete lever inventory

Every control surfaced by this analysis, with a concrete aggressive value. If
a lever is listed here it should have a number or an explicit decision — "add
a limit" without a value is not an implementable recommendation.

**Aggressive is the right default for this app.** Every value below should be
read as "start here and raise only when a real user complains." A demo app has
no legitimate heavy user to protect, so the cost of a too-tight limit is a
support conversation, while the cost of a too-loose one is an unbounded bill.

### Application layer

| # | Lever | Aggressive value | Current | Where |
|---|---|---|---|---|
| 1 | Max request body | **64 KiB** | unbounded | [L2](#l2--request-body-cap) — new middleware |
| 2 | Reject `Content-Encoding: gzip` | **yes** | accepted | [L2](#l2--request-body-cap) |
| 3 | Max title bytes | **200 B** | unbounded | [L3](#l3--field-level-validation) |
| 4 | Max content bytes | **16 KiB** anon / **32 KiB** auth | unbounded | [L3](#l3--field-level-validation) |
| 5 | Max total record bytes | **48 KiB** | none | [L3](#l3--field-level-validation) |
| 6 | Reject invalid UTF-8 | **yes** | not checked | [L3](#l3--field-level-validation) |
| 7 | Notes per identity | **10** anon / **100** auth | unbounded | [L4](#l4--per-identity-quotas) |
| 8 | Bytes per identity | **256 KiB** anon / **2 MiB** auth | unbounded | [L4](#l4--per-identity-quotas) |
| 9 | Write rate per identity | **5/min** anon / **30/min** auth | none | [L5](#l5--rate-limiting) |
| 10 | Read rate per identity | **120/min** | none | [L5](#l5--rate-limiting) |
| 11 | Rate-limiter LRU size | **10,000 UIDs** | n/a | [L5](#l5--rate-limiting) — bounds the limiter's own memory |
| 12 | `GET /api/notes` page size | **50 default, 100 max** | unbounded | [A8](#a8--read-side-amplification) |
| 13 | Title-change creates new record | **remove entirely** | creates+orphans | [A3](#a3--update-churn-amplification) |

Lever 11 deserves a note: an in-memory rate limiter keyed by UID is itself an
unbounded map an attacker can grow via [A4](#a4--identity-multiplication). Bound
it with an LRU or the mitigation becomes the vulnerability.

### Storage / retention

| # | Lever | Aggressive value | Where |
|---|---|---|---|
| 14 | Anonymous note TTL | **24 h** | [L7](#l7--retention--ttl) |
| 15 | Authenticated note TTL | **30 d** | [L7](#l7--retention--ttl) |
| 16 | TTL anchor | **creation time**, never last-modified | [A7](#a7--soft-delete-and-ttl-evasion) |
| 17 | TTL grace extension cap | **2× base**, or none at all | [TTL rules](#design-rules) |
| 18 | Deletes | **hard**, not tombstoned | [A7](#a7--soft-delete-and-ttl-evasion) |
| 19 | Tombstone purge (if soft-delete kept) | **hourly**, counted against quota until purged | [A7](#a7--soft-delete-and-ttl-evasion) |
| 20 | Sweeper interval (Cloud SQL) | **every 15 min**, batched `LIMIT 1000` | [TTL impl](#implementation) |
| 21 | Expiry filter on read | **required**, not sweeper-only | [TTL impl](#implementation) |
| 22 | DB-level size constraint | `CHECK (octet_length(content) <= 32768)` | [Datastore notes](#cloud-sql-postgres) |

### Global / platform

| # | Lever | Aggressive value | Current | Where |
|---|---|---|---|---|
| 23 | Global storage ceiling | **5 GiB** → writes return `503` | none | [L6](#l6--global-ceiling-and-kill-switch) |
| 24 | Global write-rate ceiling | **500/min** all identities | none | [L6](#l6--global-ceiling-and-kill-switch) |
| 25 | Read-only kill switch | **env var, no redeploy** | none | [L6](#l6--global-ceiling-and-kill-switch) |
| 26 | GCP budget alert | **$5 / $20 / $50** thresholds | unconfigured | [L6](#l6--global-ceiling-and-kill-switch) |
| 27 | `max_instance_count` | **3** (keep) | 3 | [cloud_run.tf:17](../infra/cloud_run.tf:17) |
| 28 | `max_instance_request_concurrency` | **10** (keep) | 10 | [cloud_run.tf:23](../infra/cloud_run.tf:23) |
| 29 | Container memory | **512 MiB** explicit | unset (default) | [L8](#l8--platform-resource-limits) |
| 30 | Container CPU | **1**, `cpu_idle = true` | unset (default) | [L8](#l8--platform-resource-limits) |
| 31 | Cloud Run request timeout | **10s** | 30s | [cloud_run.tf:24](../infra/cloud_run.tf:24) |
| 32 | `ReadTimeout` | **5s** | 15s | [server.go:173](../internal/server/server.go:173) |
| 33 | `ReadHeaderTimeout` | **2s** | **unset** | [L9](#l9--timeouts) |
| 34 | `WriteTimeout` | **10s** | 15s | [server.go:174](../internal/server/server.go:174) |
| 35 | `IdleTimeout` | **30s** | 60s | [server.go:175](../internal/server/server.go:175) |
| 36 | `chiMiddleware.Timeout` | **8s or remove** | 60s (dead) | [L9](#l9--timeouts) |

### Identity

| # | Lever | Aggressive value | Where |
|---|---|---|---|
| 37 | Anonymous Auth provider | **disabled** | [L10](#l10--firebase-side-controls) |
| 38 | Firebase App Check | **enforcing** (required if 37 stays on) | [L10](#l10--firebase-side-controls) |
| 39 | Anonymous account purge | **daily, older than 24 h** | [L10](#l10--firebase-side-controls) |
| 40 | Blocking function on `beforeCreate` | optional, if 37 stays on | [L10](#l10--firebase-side-controls) |
| 41 | Session cookie duration *(pre-migration)* | **24h**, not 168h | [L11](#l11--session-and-cookie-lifetime-pre-migration-only) |

### Observability thresholds

| # | Lever | Aggressive value | Where |
|---|---|---|---|
| 42 | Storage ceiling alert | **50%** of global (2.5 GiB) | [Observability](#observability-and-alerting) |
| 43 | Quota-rejection alert | **>100/5 min sustained** | [Observability](#observability-and-alerting) |
| 44 | Identity-creation alert | **>50/h** | [Observability](#observability-and-alerting) |
| 45 | Flat-sweeper alert | `notes_expired_total` unchanged **>2× TTL sweep interval** | [Observability](#observability-and-alerting) |

Levers **37**, **23**, **1**, and **14** carry the most weight. If only four
things ship, ship those: they respectively make identity multiplication
impossible, bound the absolute worst case, bound per-request cost, and make
anything that slips through self-cleaning.

## Recommended quota values

The per-identity subset of the [lever inventory](#complete-lever-inventory),
broken out by identity tier with the reasoning behind each number. Values here
match the inventory; this table explains them rather than restating them.

These are deliberately tight: they should be raised in response to a real user
complaining, not pre-emptively.

| Control | Anonymous (if kept) | Authenticated | Rationale |
|---|---|---|---|
| Max request body | 64 KiB | 64 KiB | Comfortably above the largest legitimate note; small enough that 10 concurrent uploads are irrelevant to memory |
| Max title | 200 B | 200 B | A title, not a payload |
| Max content | 16 KiB | 32 KiB | ~32k characters of prose; well beyond a demo note |
| Max notes per identity | 10 | 100 | 100 is generous for real use of a demo |
| Max total bytes per identity | 256 KiB | 2 MiB | The binding constraint; count × max-size is the true worst case |
| Write rate | 5/min | 30/min | A human saving fast tops out around 1/sec briefly |
| Read rate | 120/min | 120/min | Generous for a UI; bounds [A8](#a8--read-side-amplification) egress |
| List page size | 50 (max 100) | 50 (max 100) | A list call must never return unbounded rows |
| TTL | 24 h | 30 d | See below |
| Global storage ceiling | — | 5 GiB | Sized so worst case stays inside free/cheap tiers |

Worst case with these numbers, authenticated: 100 notes × 32 KiB = 3.2 MiB
per identity, capped at 2 MiB by the byte quota. One thousand real users
would be ~2 GiB — inside the global ceiling with room to spare. Note the
byte quota, not the count quota, is what binds; that is intentional, since
bytes are what gets billed.

Anonymous worst case is where it gets uncomfortable, and is exactly the
argument in [L1](#l1--identity-scarcity): 256 KiB per UID is small, but
multiplied by unbounded UIDs it is unbounded. The 5 GiB global ceiling is
what actually bounds it — meaning under a determined attack, anonymous abuse
fills the global pool and the demo goes read-only for everyone. That is the
tradeoff being accepted if anonymous writes are kept.

## TTL / retention design

Aggressive TTL was raised as acceptable, and it is the single
highest-leverage control available here. It should be adopted.

### Why TTL is disproportionately effective for this app

- It bounds total storage **regardless of how abuse happened** — including
  vectors not anticipated in this document. It is the only control here that
  is robust to its own analysis being incomplete.
- It makes abuse self-healing: a successful attack is erased within the TTL
  window without operator action.
- It costs almost nothing to implement in either candidate datastore.
- For a demo, permanent retention has no user value to protect.

### Design rules

1. **Anchor TTL to creation, not last-modification** —
   [A7](#a7--soft-delete-and-ttl-evasion). Modification-anchored TTL is
   defeated by a keep-alive loop. If a "still in use" grace period is wanted
   later, implement it as a bounded extension (e.g. up to 2× the base TTL),
   never as an open-ended reset.
2. **Shorter TTL for anonymous than authenticated.** Anonymous data is the
   most abusable and least valuable.
3. **Communicate it in the UI.** "Notes are deleted after 30 days" must be
   visible before a user writes anything — this is a demo, but silent data
   loss is still bad behavior toward real users.
4. **Expose it in the API.** Return `expires_at` on note responses so the
   frontend can display it, rather than hardcoding the policy client-side.
5. **Hard-delete on expiry.** Tombstones that accumulate forever defeat the
   purpose. If soft-delete is needed for any reason, purge tombstones on a
   schedule and count them against quota until purged.

### Recommended values

- **Anonymous notes: 24 hours.** Long enough to try the demo, short enough
  that abuse evaporates daily.
- **Authenticated notes: 30 days**, refreshed only by explicit user action if
  a keep-alive is ever added, subject to rule 1.

### Implementation

**Firestore:** native TTL policies. Add an `expires_at` timestamp field and a
TTL policy on it; Firestore deletes expired documents automatically, and
deletes are not billed as document deletes under the TTL policy. This is the
cleanest option and a strong argument for Firestore in this specific
comparison.

**Cloud SQL / Postgres:** no native TTL. Either a scheduled `DELETE FROM
notes WHERE expires_at < now()` (Cloud Scheduler → an authenticated endpoint,
or `pg_cron`), or partition by creation date and drop whole partitions —
dropping a partition is `O(1)` where a bulk `DELETE` is `O(rows)` and
generates significant WAL and vacuum load. For expected volumes a batched
scheduled delete (with `LIMIT` and a loop, to avoid one long transaction) is
simpler and sufficient.

Whichever store: **the expiry filter must also be applied on read**, not just
by the sweeper. Otherwise notes remain visible between expiry and the next
sweep, and the API's behavior depends on sweeper timing.

## Guest access: recommendation

Dropping the guest feature was offered as an option. Here is the assessment.

**Recommendation: keep anonymous access for reading, require a federated
identity (Google/GitHub) for writing.**

Reasoning:

- The guest feature's value is letting someone try the demo without signing
  up. That value is almost entirely in *seeing* the app work — which
  read-only access preserves.
- Anonymous *write* access is the root of [A4](#a4--identity-multiplication),
  the one vector that undermines every other control in this document. It
  converts every per-identity quota into a per-identity-times-unbounded
  quota.
- The cost of removal is low and the code gets *simpler*, not more complex:
  no anonymous branch, no separate anonymous quota tier, no separate
  anonymous TTL.
- It aligns with the migration's own direction. The Firebase plan already
  eliminates the guest special case in Go
  ([FIREBASE_MIGRATION_PLAN.md:60](FIREBASE_MIGRATION_PLAN.md:60)); declining
  to enable Anonymous Auth as a write identity keeps that simplification
  while closing the abuse path it otherwise inherits.

**If frictionless write access is considered essential to the demo**, use
option 2 from [L1](#l1--identity-scarcity): a single shared global pool for
all anonymous users with oldest-first eviction, plus the 24-hour TTL. This
keeps the experience and gives a hard bound. It is strictly better than
per-anonymous-UID quotas, which give the *appearance* of a bound without one.

What should **not** be done is keeping anonymous writes with per-UID quotas
and treating that as sufficient. That is the current trajectory by default,
and it is the configuration this analysis most wants to flag.

## Datastore-specific notes

The database choice is not yet made
([CLOUD_RUN_GCP_RESOURCES.md](CLOUD_RUN_GCP_RESOURCES.md#not-yet-created)
lists persistent storage as not created). Abuse-relevant differences:

### Firestore

- **For:** native TTL (see above); scales to zero cost when idle, matching
  `min_instance_count = 0`; no connection pooling concerns from Cloud Run;
  transactions make the [L4](#l4--per-identity-quotas) counter pattern
  straightforward.
- **Against:** billed **per document read/write**, so [A2](#a2--many-small-notes)
  (many small notes) hits the bill harder than raw storage size suggests.
  Rate limiting matters more here than it would with Cloud SQL.
- **Watch:** a 1 MiB per-document limit provides an incidental backstop — but
  do not rely on it; 1 MiB is far above the intended note size and it would
  surface as an opaque datastore error rather than a clean `413`.
- Firestore Security Rules are **not** a control here, since access is via the
  Go API with the service's own credentials, not direct client SDK access.
  All enforcement is application-side.

### Cloud SQL (Postgres)

- **For:** `CHECK` constraints enforce size limits at the storage layer as a
  true backstop even if application validation is bypassed or regressed;
  cheap aggregate queries for quota accounting; partitioning makes bulk
  expiry cheap.
- **Against:** no native TTL — needs a sweeper; a Cloud SQL instance bills
  continuously even when idle, unlike Firestore, which is a real
  consideration for a demo that may sit unused; connection management from a
  scale-to-zero Cloud Run service needs care.
- **Watch:** `TEXT` is unbounded — use `VARCHAR(n)` or an explicit `CHECK
  (octet_length(content) <= 32768)`. Belt-and-braces with
  [L3](#l3--field-level-validation), and it is the layer that survives an
  application-side regression.

**Leaning:** Firestore, primarily for native TTL and idle cost, provided
[L5](#l5--rate-limiting) is implemented to control per-operation billing. But
this decision has inputs beyond abuse prevention and should be made on its
own merits; this section only supplies the abuse-relevant considerations.

## Observability and alerting

You cannot respond to what you cannot see, and the current metrics
([internal/monitoring/metrics.go](../internal/monitoring/metrics.go)) are
operation-oriented — `notes_created_total`, `notes_updated_total` — with **no
gauge for stored state**. That is precisely the blind spot
[A3](#a3--update-churn-amplification) exploits: update-churn shows up as
normal update traffic while storage grows.

Add:

| Metric | Type | Why |
|---|---|---|
| `notes_stored_total` | Gauge | Actual row count — the number that matters |
| `notes_stored_bytes` | Gauge | Actual storage consumed |
| `notes_quota_rejections_total` | Counter (by reason) | An attack in progress looks like a spike here |
| `notes_identities_total` | Gauge | Sudden growth = [A4](#a4--identity-multiplication) in progress |
| `notes_expired_total` | Counter | Confirms the TTL sweeper is actually running |

The last one deserves emphasis: **a silently broken TTL sweeper is
indistinguishable from a working one until storage fills.** It is the control
this design leans on hardest and the one most likely to fail quietly. Alert
on `notes_expired_total` being *flat* — that is the signal that retention has
stopped working.

Alert on: storage above 50% of the global ceiling; identity-creation rate
above baseline; sustained quota rejections; and the flat-sweeper condition
above. Route these somewhere a human actually reads — an alert nobody sees is
equivalent to no alert.

## Implementation plan

Ordered so that each step is independently shippable and the highest-value
controls land first. Steps 1–3 are the ones that must not be skipped before
going public.

| # | Step | Depends on | Notes |
|---|---|---|---|
| 1 | Body cap ([L2](#l2--request-body-cap)) + field validation ([L3](#l3--field-level-validation)) | — | Levers 1–6. Can ship **today**, against the current code, independent of both migrations. Highest value per unit of effort. |
| 2 | Timeouts + platform resources ([L9](#l9--timeouts), [L8](#l8--platform-resource-limits)) | — | Levers 29–36. Config-only, no application logic. Also fixes the dead 60s chi timeout. |
| 3 | Decide guest posture ([L1](#l1--identity-scarcity)) | — | A decision, not code. Blocks the quota design, so make it first. |
| 4 | Firebase-side identity controls ([L10](#l10--firebase-side-controls)) | Step 3, Firebase auth | Levers 37–40. Mostly console toggles. Lever 37 is the highest-impact single change in this document. |
| 5 | Global ceiling + kill switch + budget alert ([L6](#l6--global-ceiling-and-kill-switch)) | DB choice | Levers 23–26. The backstop. Must exist before the first public request. |
| 6 | Per-identity quotas ([L4](#l4--per-identity-quotas)) | DB migration, step 3 | Levers 7–8. Transactional with the write. Denormalized counters. |
| 7 | TTL ([L7](#l7--retention--ttl)) | DB migration | Levers 14–22. Store-dependent; see [TTL design](#ttl--retention-design). Include the read-side expiry filter. |
| 8 | Rate limiting + pagination ([L5](#l5--rate-limiting), [A8](#a8--read-side-amplification)) | Firebase auth (for UID keying) | Levers 9–12. Per-instance limiter is adequate at `max_instance_count = 3`; bound the limiter's own map. |
| 9 | Metrics + alerts | Steps 5–7 | Levers 42–45, including the flat-sweeper alert. |
| 10 | Delete `generateFilename` | DB migration | Lever 13. Do not port this function; replace with a real primary key. |
| 11 | Abuse test suite | Steps 1–8 | Below. |

Steps 1 and 2 are worth emphasizing: they are **pure config and validation,
block on neither migration, and close the two vectors with the worst
severity-to-effort ratio** ([A1](#a1--large-single-note) and the memory
exhaustion behind [A5](#a5--compression-bombs-and-encoding-tricks)). There is
no reason to wait for the database work to land before shipping them.

### Testing

Each control needs a test that asserts abuse is *rejected*, not merely that
the happy path works. Quota code that silently fails open is worse than no
quota, because it produces false confidence:

- Oversized body → `413`, and the body is not persisted.
- Oversized field → `400`.
- Note count / byte quota exceeded → `429`, count does not increase.
- **Concurrent writes at the quota boundary** → quota holds. This is the test
  that catches the [L4](#l4--per-identity-quotas) race, and the one most
  likely to be omitted.
- Update with changed title → does **not** create an additional record
  ([A3](#a3--update-churn-amplification)).
- Expired note → not returned by read **and** eventually swept.
- Two identities → quotas are independent (the existing guest-isolation test
  intent, per
  [FIREBASE_MIGRATION_PLAN.md](FIREBASE_MIGRATION_PLAN.md#test-migration)).
- Gzipped request body → rejected, not silently inflated (lever 2).
- Invalid UTF-8 in title or content → `400` (lever 6).
- `GET /api/notes` with a large `limit` → clamped to the max, not honored
  (lever 12).
- Anonymous token where anonymous writes are disallowed → `403`, enforced from
  the token's `sign_in_provider` claim rather than a client flag (lever 37).
- Slow-body / slow-header request → connection closed by timeout, not held
  open (levers 32–33). Easy to omit and the only test that covers the
  duration dimension.

## Explicitly out of scope

- **Content moderation.** Notes as a host for illegal or abusive material is
  a real risk of a public writable store, but it is a different problem with
  different controls. Worth its own analysis before going public.
- **Cloud Armor / WAF.** Deliberately excluded per
  [CLOUD_RUN_DEPLOYMENT_PLAN.md](CLOUD_RUN_DEPLOYMENT_PLAN.md). That
  reasoning holds for storage abuse specifically: the controls here are
  application-layer quotas that a WAF cannot express.
- **Path traversal in `Get`/`Delete`.** Noted above as found-in-passing; it
  is a security bug, not a storage-abuse one, and the DB migration resolves
  it by construction.
- **DDoS.** Partially addressed by `max_instance_count`, but genuine
  distributed attack mitigation needs infrastructure explicitly out of scope
  for this project.
- **Backup and recovery.** Aggressive TTL and backups are in tension; for a
  demo, TTL wins and backups are unnecessary.
