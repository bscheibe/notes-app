# Cloud Run Deployment Plan

Plan for standing up `notes-app`'s API on Google Cloud Run. Scope is
deliberately limited: get the infrastructure in place and wired for
deployment, without needing the service to be reachable or to actually serve
traffic yet, and without migrating auth providers as part of this work (see
[FIREBASE_MIGRATION_PLAN.md](FIREBASE_MIGRATION_PLAN.md) for that).

## Three questions this plan answers

### 1. Can the API be internal-only / not exposed to the open internet?

**No, not at the network layer, and it doesn't need to be.**

Cloud Run's `ingress: internal` setting only allows traffic from an internal
Application Load Balancer, resources inside the same project's VPC/Shared
VPC, VPC Service Controls perimeters, and a specific allowlist of Google
Cloud services (Cloud Scheduler, Tasks, Eventarc, Pub/Sub, Workflows, etc.) -
[confirmed against Cloud Run's ingress
docs](https://docs.cloud.google.com/run/docs/securing/ingress). **Firebase
Hosting is not on that allowlist.**

This is moot for `notes-app` regardless: `notes-webpage` calls the API
directly from the browser via `fetch()` with a bearer token
([js/api.js](../../notes-webpage/js/api.js)) - there is no Firebase Hosting
rewrite proxy in this architecture (`firebase.json` has no `rewrites`
config). A browser is an arbitrary internet client, not a GCP-internal
resource, so no ingress setting can distinguish "a browser with a valid
token" from "the open internet" at the network layer. The service must run
with `ingress: all`.

"Not exposed to the open internet" therefore has to mean **authenticated**,
not **network-isolated**. Every `/api/*` request already requires a verified
Firebase ID token per the Firebase migration plan - that is the actual
access boundary, enforced in the application, not the network.

### 2. How does GCP handle "security groups," and what's the idiomatic way to automate/secure it?

GCP has no direct equivalent to AWS security groups for Cloud Run. Access
control is two independent layers:

- **Ingress** - the network path ("can a request reach this at all"):
  `all` / `internal` / `internal-and-cloud-load-balancing`.
- **IAM invoker** (`roles/run.invoker`) - the authorization right ("is this
  caller allowed to invoke it"), independent of network path.

For this service: `ingress: all` + IAM invoker `allUsers` (publicly
invokable), with the real gate being Firebase ID token verification inside
the Go app itself.

The idiomatic way to keep the automation around this secure and up to date,
without manually managed long-lived credentials:

- **Workload Identity Federation (WIF)** instead of a downloaded
  service-account JSON key in GitHub Secrets. GitHub Actions' OIDC token is
  exchanged for a short-lived GCP credential at run time - nothing stored,
  nothing to rotate, nothing that can leak from a repo secret dump.
- **Least-privilege service accounts** - a dedicated deploy service account
  scoped to exactly `run.admin` (on this service), `iam.serviceAccountUser`,
  and `artifactregistry.writer`, not a broad `Editor`/`Owner` role.
- **Dependabot** (already configured, see
  [.github/dependabot.yml](../.github/dependabot.yml)) keeps the GitHub
  Actions implementing this (`google-github-actions/auth`,
  `google-github-actions/deploy-cloudrun`) patched.

Known tradeoff, noted rather than avoided: both of those Google-published
Actions score ~6/10 on StepSecurity with 25-31 flagged vulnerabilities each
in their transitive npm dependency trees - the same pattern seen across
every Node.js-based Action evaluated during this project's CI/CD work, including
Google's own. Unlike the earlier semver-tagging case, there's no native-binary
alternative for exchanging a GitHub OIDC token for a GCP credential, so this
is accepted rather than worked around.

### 3. What infrastructure is needed to stand this up?

See task list below. Kept intentionally minimal: enough to deploy, not a
full production topology (no custom domain, no autoscaling tuning, no
alerting - those are follow-ups once the service is actually meant to serve
traffic).

## Tasks

| # | Task | Notes |
|---|---|---|
| 1 | Bootstrap Terraform config and Infrastructure Manager | New `infra/` directory in this repo: `provider "google"`, backend/state managed by Infrastructure Manager (`gcloud infra-manager deployments apply`, not local/self-hosted state). See Tooling choice below for why. |
| 2 | Enable required GCP APIs via Terraform | `google_project_service` for `run.googleapis.com`, `artifactregistry.googleapis.com`, `iamcredentials.googleapis.com`. Declarative, not a manual `gcloud services enable` step. |
| 3 | Create an Artifact Registry Docker repository | `google_artifact_registry_repository`. Standardize the deploy source on Artifact Registry (native Cloud Run integration) rather than pulling from GHCR at deploy time. |
| 4 | Create a dedicated deploy service account | `google_service_account` + `google_project_iam_member` grants, least privilege: `roles/run.admin` (scoped to the service once it exists), `roles/iam.serviceAccountUser`, `roles/artifactregistry.writer`. No broad `Editor`/`Owner`. |
| 5 | Set up Workload Identity Federation | `google_iam_workload_identity_pool` + `google_iam_workload_identity_pool_provider`, trust condition restricted to `bscheibe/notes-app`, bound to the deploy service account from #4. This is the actual "automate + keep secure" answer to question 2 - zero long-lived keys in GitHub Secrets. |
| 6 | Create the Cloud Run service | `google_cloud_run_v2_service`: `ingress: all`, IAM invoker `allUsers` (`google_cloud_run_v2_service_iam_member`), `min-instances: 0` (no need to stay warm - not serving traffic yet). App-layer Firebase token verification does the real access gating once that migration lands. `max-instances` set per task #9 in the same resource, not added later. |
| 7 | Add a deploy workflow | New `deploy.yml` (or extend `release.yml`), using `google-github-actions/auth` (WIF) to authenticate, then running `terraform apply` (or `gcloud infra-manager deployments apply`) against the config from #1. Gated the same way the existing `binary`/`image` jobs are. Wire it up; don't need to actually trigger/run it yet. |
| 8 | Decide image source for the Cloud Run service | Either the Artifact Registry repo from #3, or point directly at the already-published `ghcr.io/bscheibe/notes-app` image. Decide before #7 is finalized. |
| 9 | Set a strict cost-surge cap on the service | `max-instances` set low (single digits) rather than Cloud Run's default of 100 - the real lever against a cost blowout from a traffic spike or abuse against the public, auth-gated endpoint. Pair with a conservative per-instance `concurrency` setting and a short request `timeout`. Once capped, excess requests during a surge get `429`s instead of Cloud Run scaling (and billing) without bound. Cheap, Cloud Run-native, no new infrastructure - unlike Cloud Armor (see below), this requires no load balancer/NEG topology. |

## Explicitly out of scope / flagged, not a task yet

**Local-disk note storage is incompatible with Cloud Run.**
`internal/repository/note_repository.go` stores notes as markdown files on
local disk (`filepath.Join`, `os.MkdirAll`, keyed per-user directory).
Cloud Run's local filesystem is ephemeral - wiped on every new
instance/revision/scale-to-zero cycle. The service can be *stood up* and
will run under this plan, but any notes written to it will be lost. Real
persistence (Cloud Storage, Firestore, or Cloud SQL) is a prerequisite
before this deployment actually serves production traffic - it is not a
prerequisite for standing up the infrastructure itself, which is all this
plan covers.

**Auth provider migration** (Firebase ID token verification middleware,
removing the current OAuth/cookie-session code) is tracked separately in
[FIREBASE_MIGRATION_PLAN.md](FIREBASE_MIGRATION_PLAN.md) and is not part of
this plan.

**Cloud Armor / a WAF layer was considered and deliberately not included.**
For this app's actual shape - a JSON API with no HTML-rendering surface, no
payments, no PII beyond what Firebase Auth already holds, personal-scale
traffic - a WAF's main value (OWASP rule sets for SQLi/XSS/RCE) protects
against attack classes this API mostly can't be vulnerable to, and
geo-blocking has no clear use case here. The piece of Cloud Armor that
*would* matter - rate limiting against cost/abuse surges - is available far
more cheaply via task #9 above (`max-instances`/concurrency caps), without
standing up a serverless NEG + external load balancer + SSL cert + DNS, and
without the ingress change (`all` -> `internal-and-cloud-load-balancing`)
that a load balancer in front of Cloud Run would require. Revisit if the
app's threat profile changes materially: real user growth where abuse
becomes plausible at a scale `max-instances` can't absorb gracefully, a
custom domain (which wants a load balancer anyway, making the NEG/LB cost
already-paid), or genuine DDoS exposure.

## Tooling choice

**Terraform (`google` provider), deployed via Google Cloud Infrastructure
Manager**, rather than raw `gcloud` CLI commands. Infrastructure Manager
runs the Terraform apply as a managed GCP service - it hosts the state
backend and execution, so there's no self-hosted state bucket/locking or a
Terraform-runner step to build in CI beyond authenticating and triggering
it. This gives declarative config, `plan` review, and drift detection for
the GCP resources in this plan without taking on Terraform's usual
operational overhead of managing state yourself.

This is a departure from how repo-level config (Dependabot, branch
ruleset) was applied directly via API/CLI earlier in this project - that
was for one-off GitHub settings, not a growing set of interdependent cloud
resources (APIs, Artifact Registry, service accounts, WIF, the Cloud Run
service itself) where `plan`/drift-detection has real value.

**Why not a GCP-native alternative to CloudFormation instead:** looked for
one specifically to avoid the Terraform provider dependency. There isn't a
real one as of this plan:

- **Deployment Manager** was GCP's actual CloudFormation-equivalent (native
  YAML/Jinja/Python templates, no external provider) but **reached end of
  support on March 31, 2026** - not viable going forward.
- Its official replacement, **Infrastructure Manager**, is not a native
  alternative - it's Terraform itself, with Google hosting the state
  backend and running `apply` for you. The `provider "google"` HCL
  dependency is unavoidable either way; what Infrastructure Manager removes
  is the self-hosted-backend/CI-runner burden, which is the part worth
  having for this plan.
- **Config Connector** is a genuinely different (Kubernetes-native,
  CRD-based) model, but requires a GKE cluster to run at all - not
  applicable, since this project isn't on GKE and provisioning a cluster
  just to run Config Connector would be a bigger footprint than what it's
  meant to avoid.
