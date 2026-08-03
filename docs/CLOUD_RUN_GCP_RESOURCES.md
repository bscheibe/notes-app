# GCP Resources Created

Inventory of every GCP resource created while implementing the
[Cloud Run Deployment Plan](CLOUD_RUN_DEPLOYMENT_PLAN.md), applied to
project `notes-app-gcp-504419` (region `us-central1`).

Split across two layers - see
[CLOUD_RUN_CICD_BOOTSTRAP.md](CLOUD_RUN_CICD_BOOTSTRAP.md) for the
rationale:

- **CI/CD plumbing** - APIs, the deploy service account and its IAM roles,
  Workload Identity Federation, the Artifact Registry repo - created once,
  by hand, via `gcloud`. Not Terraform-managed.
- **The application itself** - the Cloud Run service and its IAM binding -
  managed by Terraform in [`infra/`](../infra), applied manually
  (`terraform apply`, run by a human). Not applied by CI, not via
  Infrastructure Manager.

## Resources

| Resource | Managed by | Why it exists |
|---|---|---|
| 6x API enablement (`run`, `artifactregistry`, `iamcredentials`, `iam`, `config`, `cloudresourcemanager`) | `gcloud services enable` | Prerequisite for every resource below. |
| Deploy service account (`notes-app-deploy@...`) | `gcloud iam service-accounts create` | Identity GitHub Actions assumes (via WIF, not a downloaded key) to deploy. |
| Deploy SA's project IAM roles (`run.admin`, `iam.serviceAccountUser`, `artifactregistry.writer`, `config.agent`, `iam.workloadIdentityPoolAdmin`) | `gcloud projects add-iam-policy-binding` | Least-privilege set for deploying images and updating the Cloud Run service - no project-wide `Editor`/`Owner`. |
| Workload Identity Pool + Provider (`github-actions`) | `gcloud iam workload-identity-pools create` / `providers create-oidc` | Lets GitHub Actions' OIDC token exchange for a short-lived GCP credential, restricted to `bscheibe/notes-app` via `attribute_condition`. Zero long-lived keys in GitHub Secrets. |
| WIF impersonation binding on the deploy SA | `gcloud iam service-accounts add-iam-policy-binding` | Lets the WIF pool actually assume the deploy SA's identity. |
| Artifact Registry Docker repository (`notes-app`) | `gcloud artifacts repositories create` | Deploy source for the Cloud Run service. Chosen over pulling from GHCR at deploy time for native Cloud Run integration (task 8 of the plan). |
| Cloud Run service (`notes-app`) | Terraform, [`infra/cloud_run.tf`](../infra/cloud_run.tf) | The API itself. `ingress: all`, `min_instance_count: 0`, `max_instance_count: 3`, currently running Google's public placeholder image (`us-docker.pkg.dev/cloudrun/container/hello`) - no notes-app code is deployed to it yet. **No public invoker binding** - only the deploy service account can invoke it right now (see standby comment in `cloud_run.tf`). |
| `run.admin` on the Cloud Run service, scoped to the deploy SA | Terraform, [`infra/deploy_service_account.tf`](../infra/deploy_service_account.tf) | Lets the deploy workflow update the service's image on each release, scoped to this one service rather than project-wide. |

Live at: `https://notes-app-zam6wpkcba-uc.a.run.app`

## Not yet created

- No custom domain, load balancer, or Cloud Armor/WAF (deliberately out of
  scope - see the plan's "Explicitly out of scope" section).
- No persistent storage (Cloud Storage/Firestore/Cloud SQL) - the plan
  flags this as a blocker before the service can actually hold real data,
  separate from standing up the infrastructure itself.
- No real application image - the Cloud Run service is running Google's
  placeholder, not `notes-app`. The Go code hasn't been migrated to the
  Firebase-auth JSON API yet (tracked in
  [FIREBASE_MIGRATION_PLAN.md](FIREBASE_MIGRATION_PLAN.md)), so there's
  nothing meaningful to deploy yet regardless.

## Where cost could come from

| Resource | Billing model | Current exposure |
|---|---|---|
| Cloud Run service | Per-request CPU/memory + a small per-revision storage fee. `min_instance_count: 0` means **zero compute charge while idle**. | Only the deploy service account can invoke it right now - no `allUsers` binding - so it isn't reachable by an arbitrary caller. `max_instance_count: 3` + concurrency 10 + 30s timeout (task 9) caps the ceiling regardless, once it is made public. |
| Artifact Registry | Storage per GB-month + network egress on pulls. Empty until the first release pushes an image. | None yet. Small, storage-only, once populated. |
| Workload Identity Pool/Provider, service account, IAM bindings | Free. GCP does not charge for IAM resources. | None, ever. |
| API enablement | Free to enable. | None. |

**Not publicly reachable right now.** The service is `ingress: all` (per
the plan's reasoning in question 1: Firebase Hosting can't be
distinguished from the open internet at the network layer regardless),
but there is no `allUsers` invoker binding, so only the deploy service
account can invoke it. This will need revisiting once Firebase auth
verification is actually deployed and `allUsers` is added as the real
access gate (see the standby comment in
[`infra/cloud_run.tf`](../infra/cloud_run.tf)).

## Standby state

Current configuration is already the lowest-cost state without deleting
resources outright: `min_instance_count: 0`, no public invoker binding, an
empty Artifact Registry repo. Optional belt-and-suspenders, not yet
configured: a [budget
alert](https://cloud.google.com/billing/docs/how-to/budgets) on the
billing account at a low threshold, so any unexpected spend surfaces by
email instead of running silently.
