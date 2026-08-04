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

All 9 tasks below are implemented and have been applied to the real
`notes-app-gcp-504419` project. Scope ended up split across two layers, not
one - see [CLOUD_RUN_CICD_BOOTSTRAP.md](CLOUD_RUN_CICD_BOOTSTRAP.md) for why:
CI/CD plumbing (APIs, the deploy service account and its IAM roles, WIF, the
Artifact Registry repo) is one-time setup applied by hand via `gcloud`, not
Terraform. Only the Cloud Run service itself is Terraform-managed, in
[`infra/`](../infra), applied manually (`terraform apply` run by a human) -
not by CI, and not via Infrastructure Manager on every release. See
[CLOUD_RUN_GCP_RESOURCES.md](CLOUD_RUN_GCP_RESOURCES.md) for the full
resource inventory.

| # | Task | Status | Notes |
|---|---|---|---|
| 1 | Bootstrap Terraform config | Done | [`infra/`](../infra): `provider.tf`, `versions.tf`, `variables.tf`. State is local, applied manually by a human - no CI-triggered apply, no Infrastructure Manager. |
| 2 | Enable required GCP APIs | Done | `run.googleapis.com`, `artifactregistry.googleapis.com`, `iamcredentials.googleapis.com`, `iam.googleapis.com` (the last needed for the service account + WIF resources in #4/#5) - enabled via `gcloud services enable`, one time, per [CLOUD_RUN_CICD_BOOTSTRAP.md](CLOUD_RUN_CICD_BOOTSTRAP.md). |
| 3 | Create an Artifact Registry Docker repository | Done | Created via `gcloud artifacts repositories create`, per [CLOUD_RUN_CICD_BOOTSTRAP.md](CLOUD_RUN_CICD_BOOTSTRAP.md). Deploy source standardized on Artifact Registry; `release.yml`'s `image` job pushes there in addition to GHCR. |
| 4 | Create a dedicated deploy service account | Done | Created via `gcloud iam service-accounts create`, per [CLOUD_RUN_CICD_BOOTSTRAP.md](CLOUD_RUN_CICD_BOOTSTRAP.md). `roles/run.admin` scoped to the `notes-app` service specifically (via [`infra/deploy_service_account.tf`](../infra/deploy_service_account.tf)'s `google_cloud_run_v2_service_iam_member`, not a project-level grant), plus `roles/iam.serviceAccountUser` and `roles/artifactregistry.writer` at the project level. |
| 5 | Set up Workload Identity Federation | Done | Pool + OIDC provider created via `gcloud iam workload-identity-pools`, per [CLOUD_RUN_CICD_BOOTSTRAP.md](CLOUD_RUN_CICD_BOOTSTRAP.md), `attribute_condition` restricted to `assertion.repository == "bscheibe/notes-app"`, bound to the deploy service account. Zero long-lived keys in GitHub Secrets - the 4 repo secrets (`GCP_WORKLOAD_IDENTITY_PROVIDER`, `GCP_DEPLOY_SERVICE_ACCOUNT`, `GCP_PROJECT_ID`, `GCP_REGION`) are resource identifiers, not credentials. |
| 6 | Create the Cloud Run service | Done | [`infra/cloud_run.tf`](../infra/cloud_run.tf): `google_cloud_run_v2_service`, `ingress: INGRESS_TRAFFIC_ALL`, `min_instance_count: 0`. Deployed with Google's public placeholder image (`us-docker.pkg.dev/cloudrun/container/hello`) initially - Terraform's `image` field has `lifecycle.ignore_changes` so it won't fight `deploy.yml`'s per-release image updates. **No public invoker binding yet** - only the deploy service account can invoke it until Firebase auth verification is the real access gate (see `cloud_run.tf`'s standby comment). |
| 7 | Add a deploy workflow | Done | [`.github/workflows/deploy.yml`](../.github/workflows/deploy.yml): triggers on `workflow_run` of `release` completing successfully, authenticates via WIF, runs `gcloud run deploy --image=...` to roll out the release's actual image onto the already-existing service (resolved from the git tag on the triggering commit, since `workflow_run` doesn't expose the other workflow's job outputs directly). Does not touch infrastructure - that's a separate, manual `terraform apply`. |
| 8 | Decide image source for the Cloud Run service | Done | **Artifact Registry**, not GHCR - native Cloud Run integration, no cross-registry pull auth needed. `release.yml`'s `image` job pushes to both; Cloud Run only ever pulls from Artifact Registry. |
| 9 | Set a strict cost-surge cap on the service | Done | Set in the same `google_cloud_run_v2_service` resource as #6: `max_instance_count: 3`, `max_instance_request_concurrency: 10`, `timeout: "30s"`. Once capped, excess requests during a surge get `429`s instead of unbounded scaling and billing. |

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

**Terraform (`google` provider)** for the Cloud Run service, applied
manually by a human, not via CI and not via Infrastructure Manager. This
gives declarative config, `plan`/preview review, and drift detection for
the one resource that's worth having that for (the Cloud Run service and
its IAM binding) without introducing a CI-triggered identity capable of
creating or modifying cloud resources on a merged PR.

Infrastructure Manager (Google-hosted Terraform state + execution) was
tried first, per an earlier version of this plan. It was dropped after
repeated friction: its state is entirely separate from any local
Terraform state, so every fresh `deployments apply` attempts to `create`
all resources from scratch and needs either broad create-permission IAM
grants or resource-by-resource state seeding to reconcile against
already-existing infrastructure; several resource types (e.g.
`google_iam_workload_identity_pool`) aren't supported by its
`--import-existing-resources` auto-import path at all. None of that
complexity is worth it for a single-resource Terraform root applied by
hand. `gcloud infra-manager previews create` (Infra Manager's `plan`
equivalent) is still used as a one-off dry-run check before an apply, but
Infra Manager does not own state or run the apply itself.

CI/CD plumbing (the deploy service account, its IAM roles, WIF, the
Artifact Registry repo) isn't in Terraform's scope at all - see
[CLOUD_RUN_CICD_BOOTSTRAP.md](CLOUD_RUN_CICD_BOOTSTRAP.md) for why it's
one-time `gcloud` setup instead.

**Why not a GCP-native alternative to CloudFormation instead:** looked for
one specifically to avoid the Terraform provider dependency. There isn't a
real one as of this plan:

- **Deployment Manager** was GCP's actual CloudFormation-equivalent (native
  YAML/Jinja/Python templates, no external provider) but **reached end of
  support on March 31, 2026** - not viable going forward.
- **Infrastructure Manager**, its official replacement, is not a native
  alternative either - it's Terraform itself, with Google hosting the state
  backend and running `apply` for you (see above for why that hosting
  wasn't worth it here). The `provider "google"` HCL dependency is
  unavoidable either way.
- **Config Connector** is a genuinely different (Kubernetes-native,
  CRD-based) model, but requires a GKE cluster to run at all - not
  applicable, since this project isn't on GKE and provisioning a cluster
  just to run Config Connector would be a bigger footprint than what it's
  meant to avoid.
