# Firebase Migration Plan

This document describes the plan to split the Notes App into two repos and move
hosting/auth to Firebase:

- **[notes-app](https://github.com/bscheibe/notes-app)** (this repo) becomes a
  pure JSON API — no server-rendered HTML, no OAuth handling, no cookie
  sessions.
- **[notes-webpage](https://github.com/bscheibe/notes-webpage)** is a new,
  separate static site deployed to Firebase Hosting, using Firebase Auth
  (client-side) for sign-in and calling notes-app as a JSON API.

`notes-webpage` has been scaffolded (static HTML/CSS/JS, `firebase.json`,
`.firebaserc`, placeholder Firebase config) but is **not yet functional** — it
depends on the Go API rewrite described here, which has not been implemented
yet. This document is the plan for that remaining work.

A small piece of file-by-file disposition below has already landed ahead of
the rest, as a standalone cleanup rather than part of the auth rewrite:
`templates/` has moved to `notes-webpage`, and `note_handler.go` returns JSON
instead of rendering HTML. **None of the actual auth migration has
happened** — `internal/auth/`, `internal/middleware/auth_middleware.go`,
`internal/handlers/auth_handler.go`, and `models.User`/`GuestSession`/
`Identity`/`Provider` are all still in place and still working exactly as
described below, and the JSON responses from `note_handler.go` are not yet
the `/api/*` surface this document specifies (no route prefix, no Firebase
token verification, no CORS). See the note on `note_handler.go` in the
disposition table.

## Why

The current app has no client-side JavaScript and no JSON API. Every page is
rendered server-side with `html/template` (`templates/index.html`,
`templates/login.html`), and OAuth (Google/GitHub) plus guest sessions are
handled entirely in Go using `golang.org/x/oauth2` and `gorilla/sessions`
cookie sessions. Firebase Hosting only serves static files — it cannot run
server logic — so this architecture has to change before the frontend can move
there. Firebase Auth's client-side SDK can run the entire OAuth flow (and
anonymous/guest auth) in the browser, which removes the need for Go to handle
OAuth or sessions at all once the frontend calls the API with a Firebase ID
token instead of a cookie.

## Current state (as of this document)

- One Go repo (`notes-app`) does routing, OAuth, sessions, and note storage.
- `internal/handlers/note_handler.go` returns JSON, driven by the same
  `<form>`-style POST body (`title`/`content`/`original_filename`) as before —
  this is not yet the `/api/*` surface below; it's an interim state ahead of
  the actual rewrite (no route prefix, no Firebase token verification, no
  `DELETE`, no `PageData`/`html/template` since `templates/` moved out).
- `internal/handlers/auth_handler.go` + `internal/auth/` handle OAuth
  initiation/callback, guest sessions, and login/logout, backed by
  `gorilla/sessions` cookies and a filesystem-backed user repository.
- `internal/middleware/auth_middleware.go` reads the cookie session on every
  request (`RequireAuth`/`OptionalAuth`) to identify the user or guest.
- `internal/repository/note_repository.go` stores notes as markdown files
  under a per-user directory, keyed by a `userID string` — this key is
  currently either an OAuth user's ID or a guest's session-scoped UUID.
- No JSON API exists anywhere in the app.

## Target end state

- `notes-app`: pure JSON API under `/api/*`. No `html/template`, no
  `templates/` directory, no cookie sessions, no OAuth handling. Every `/api/*`
  request must carry a verified Firebase ID token
  (`Authorization: Bearer <token>`) — there is no unauthenticated/optional-auth
  path. Health/metrics endpoints (`/health`, `/healthz`, `/livez`, `/metrics`)
  remain unauthenticated and outside `/api`.
- `notes-webpage`: static SPA on Firebase Hosting. Firebase Auth's JS SDK
  handles Google sign-in, GitHub sign-in, and anonymous (guest) sign-in
  entirely client-side. The app calls notes-app's JSON API, attaching the
  current user's Firebase ID token on every request.
- **Guests are not a special case in Go.** Firebase Anonymous Auth issues a
  real, stable Firebase UID for guests just like it does for signed-in users.
  Every request — anonymous or not — carries a verified ID token with a UID.
  `note_repository.go`'s `getUserDirectory(userID)` keeps working unchanged;
  it's simply fed by the token's `sub`/UID claim instead of a cookie session
  ID. This means the Go side never needs an `OptionalAuth`/guest branch, and
  `models.User`, `models.GuestSession`, `models.Identity`, `models.Provider`
  all become unnecessary. If the frontend wants to show a "Guest" badge, it
  checks `firebase.auth().currentUser.isAnonymous` client-side — no server
  involvement.
- **Single Firebase project** is shared across local/staging/prod for now
  (not one project per environment). This can be revisited later if stronger
  environment isolation is needed.

## Target Go API surface

All routes below require a valid Firebase ID token; unauthenticated requests
get `401`.

| Method | Path | Body | Response |
|---|---|---|---|
| `GET` | `/api/notes` | — | `200 { "notes": [{ "filename", "title", "created", "modified" }, ...] }` |
| `GET` | `/api/notes/{filename}` | — | `200 { "filename", "title", "content", "created", "modified" }`, `404` if not found |
| `POST` | `/api/notes` | `{ "title", "content" }` | `201 { note }` |
| `PUT` | `/api/notes/{filename}` | `{ "title", "content" }` | `200 { note }` |
| `DELETE` | `/api/notes/{filename}` | — | `204` |

This replaces the current single `POST /notes/` handler that overloads
create-vs-update behind a hidden `original_filename` form field. The filename
now comes from the URL path on update, which is a cleaner REST shape and
removes that overload. `DELETE` is a new route — `note_service`/`note_repository`
already have delete logic that was never wired to an HTTP route; adding it
here is a small scope addition but makes the JSON API complete.

CORS is required since the API and the static site are different origins.
Use `github.com/go-chi/cors` (the standard chi-ecosystem CORS middleware),
mounted once in `server.go`, allowing the configured Hosting origin, methods
`GET/POST/PUT/DELETE/OPTIONS`, and the `Authorization`/`Content-Type` headers.
`AllowCredentials` should be `false` — auth is via the `Authorization` header
(bearer token), not cookies, so there's no credentialed-CORS complexity.

## Firebase token verification in Go

Use `firebase.google.com/go/v4`'s `auth.Client.VerifyIDToken`, not a
hand-rolled JWKS fetch/verify. It's the officially supported approach and
handles Google's key rotation and caching automatically — reimplementing that
is unnecessary complexity for what a small self-hosted API needs.

Planned structure: a new `internal/firebaseauth/` package replacing
`internal/middleware/auth_middleware.go` (which is deleted):

- `verifier.go` — wraps `auth.Client`, constructed once at startup from
  `firebase.NewApp(ctx, &firebase.Config{ProjectID: cfg.Firebase.ProjectID})`.
  Verification only needs the project ID, not a service-account key, for
  `VerifyIDToken`.
- `middleware.go` — a single `RequireAuth` middleware (no `OptionalAuth`):
  reads `Authorization: Bearer <token>`, verifies it, and on success stores
  the token's UID (and any needed claims like email/name) in request context.
  On failure it returns a JSON `401 {"error": "..."}` — not a redirect, since
  this is now an API, not a page.
- To keep unit tests independent of real Firebase/Google JWKS calls, define a
  small `TokenVerifier` interface (`VerifyIDToken(ctx, token) (*auth.Token,
  error)`) so tests can inject a fake verifier instead of hitting the network.

## File-by-file disposition (notes-app)

**Delete:**
- `internal/auth/` — entire package (OAuth client, auth service, filesystem
  user repository) and its tests. Firebase owns user identity now; there's no
  local user store to maintain.
- `internal/handlers/auth_handler.go` and its tests
  (`auth_handler_test.go`, `auth_handler_integration_test.go`).
- `internal/middleware/auth_middleware.go` and `auth_middleware_test.go`.
- `internal/models/user.go` (`User`, `GuestSession`, `Identity`, `Provider`).

**Already done, ahead of the rest of this plan:**
- `templates/` (`index.html`, `login.html`) — moved to `notes-webpage`.
- `internal/handlers/note_handler.go` — dropped `html/template`, `PageData`,
  `UserInfo`; handlers now use `encoding/json`. Still missing from this task:
  the `/api/*` route prefix, `DELETE`, and Firebase token verification — those
  land with the rest of this plan, below.
- `internal/server/server.go` — dropped the `/static/*` file server (it
  served no directory that existed in this repo).

**Rewrite (remaining):**
- `internal/server/server.go` — drop auth handler/service/user-repo wiring;
  mount CORS; mount the new `firebaseauth.RequireAuth` middleware; replace the
  route table with the `/api/*` tree above; drop the `/` route in favor of
  `/api/notes`.
- `internal/config/config.go` — remove the entire `Auth` struct
  (`Google`/`GitHub`/`Session`/`GuestSessionDuration`) and
  `GetSessionDuration`/`GetGuestSessionDuration`/`generateRandomSecret`; add:
  ```go
  Firebase struct {
      ProjectID string
  }
  CORS struct {
      AllowedOrigin string
  }
  ```
- `config.local.yaml`, `config.staging.yaml`, `config.prod.yaml` — replace the
  `auth:` block in each with:
  ```yaml
  firebase:
    project_id: "<shared-firebase-project-id>"
  cors:
    allowed_origin: "<hosting-url-for-this-environment>"
  ```
- `go.mod` — remove `golang.org/x/oauth2`, `gorilla/sessions` (and the
  now-unused indirect `gorilla/securecookie`); add `firebase.google.com/go/v4`
  and `github.com/go-chi/cors`. Confirm whether `github.com/google/uuid` is
  still needed anywhere once the old auth/middleware code is gone.

**Keep as-is:**
- `internal/service/note_service.go`, `internal/repository/note_repository.go`
  — already keyed by a plain `userID string`, agnostic to where that ID came
  from. No signature changes needed.
- `internal/models/note.go` (`Note`, `CreateNoteRequest`, `NoteList`) — already
  has `json:` tags, fits directly as API request/response bodies.
- `internal/monitoring/*` — unrelated to auth.

## Test migration

- Delete the OAuth/cookie-session test files listed above (they test code
  that no longer exists).
- Add Go-level API integration tests (e.g.
  `internal/handlers/note_handler_integration_test.go`, following the
  `httptest` pattern from the old `auth_handler_integration_test.go`) driving
  `GET/POST/PUT/DELETE /api/notes...` with a fake/injected UID, replacing what
  the current Playwright suite verifies via form POSTs against rendered HTML.
  A guest-isolation test becomes: two different fake UIDs → confirm disjoint
  note lists — this preserves the intent of the guest session-isolation tests
  that existed against the old cookie middleware (dropped as WIP during this
  planning session since they tested code being deleted), just expressed
  against Firebase UIDs instead of session cookies.
- Move `tests/e2e/*.spec.ts`, `tests/pages/*.ts`, `playwright.config.ts`,
  `tests/global-setup.ts`/`global-teardown.ts`, and the Playwright-related
  `package.json` entries to `notes-webpage`. Rewrite them to drive the static
  SPA plus the Firebase Auth Emulator (`firebase emulators:start --only auth`)
  instead of real OAuth providers. Keep CSS class/element names consistent
  between the old templates and the new `notes-webpage/index.html` (already
  done in the initial scaffold) to minimize selector rewrites in
  `tests/pages/HomePage.ts`.

## Sequencing for the implementation pass

This document only covers planning — the following is the recommended order
for the actual implementation work, kept separate so the API rewrite can be
reviewed on its own:

1. ~~Rewrite `note_handler.go` to JSON.~~ Done, ahead of the rest of this
   plan — see the disposition table above. Still owes this plan the
   `/api/*` prefix and the new `DELETE` route, picked up in step 5 below.
2. ~~Move the Playwright suite into `notes-webpage`; drop `templates/` and
   the Playwright/TS tooling (`playwright.config.ts`, `tsconfig.json`,
   `package.json`) from `notes-app`.~~ Done, ahead of the rest of this plan.
   The suite still drives the old form-POST flow against `notes-webpage`'s
   reference copy; rewriting it to drive the SPA plus the Firebase Auth
   Emulator (per the Test migration section above) is still outstanding.
3. Add `firebase.google.com/go/v4` + `github.com/go-chi/cors` to `go.mod`;
   remove `oauth2`/`gorilla/sessions`.
4. Add `Firebase`/`CORS` config fields; update the three `config.*.yaml`
   files with a real (or placeholder) shared project ID.
5. Build `internal/firebaseauth/` (verifier + middleware + context helpers)
   with unit tests against an injectable `TokenVerifier`.
6. Finish `note_handler.go`: add the `/api/*` prefix and wire the new
   `DELETE` route.
7. Rewrite `server.go`: drop old auth wiring, mount CORS + `RequireAuth`,
   register `/api/*` routes.
8. Delete `internal/auth/`, `internal/handlers/auth_handler*.go`,
   `internal/middleware/auth_middleware*.go`, `internal/models/user.go`.
9. Write/port Go API integration tests; confirm `go build ./...` and
   `go test ./...` are green.
10. Manually verify the API against a real Firebase ID token (e.g. via a
    scratch HTML page hitting the Auth JS SDK, or `firebase auth:sign-in`) to
    confirm token verification works end-to-end, not just against mocks.
11. Point `notes-webpage/js/firebase-config.js` at the real Firebase project
    and confirm `js/api.js`/`js/auth.js` work against the live API locally.
12. Finish rewriting the Playwright suite (from step 2) against the live
    `/api/*` surface and the Firebase Auth Emulator.
13. Final cleanup: grep for leftover references to `models.User`,
    `models.Provider`, `models.GuestSession`, `models.Identity` across the
    repo, and confirm `go.mod`/`go.sum` have no unused deps.

## Explicitly out of scope for the current round

The `notes-webpage` scaffold and this document were produced without making
any of the Go auth changes above. `internal/auth/` and
`internal/middleware/auth_middleware.go` still exist and still work as
before — OAuth, cookie sessions, and guest sessions are all still live. The
goal of this round was two repos in place and an unambiguous plan for the
remaining work, not the auth rewrite itself. The `templates/` move and the
`note_handler.go` JSON conversion happened as a standalone prune (see the
disposition table above), not as part of executing this plan.
