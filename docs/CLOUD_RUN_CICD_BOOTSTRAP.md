# CI/CD bootstrap (one-time, manual)

These resources back the GitHub Actions deploy workflow itself: the identity
it authenticates as, its permissions, and the Docker registry it pushes to.
They're created **once, by hand**, with a human's own broad `gcloud`
credentials — never by Terraform, never by Infrastructure Manager, and
never by CI. A CI-triggered identity must never be able to grant itself
IAM, so nothing that touches the deploy service account's own identity can
be something CI or an automated apply creates.

Project: `notes-app-gcp-504419` ("Notes-App-GCP"), region `us-central1`.

Run once. Re-run an individual command only if that specific piece needs
to change (a new API, a new role, a new repo). The Cloud Run service
itself is **not** part of this bootstrap — it's managed by Terraform in
[`infra/`](../infra), applied manually by a human (`terraform apply`),
never by CI and never via Infrastructure Manager.

## 1. Enable required project APIs

```bash
gcloud services enable \
  run.googleapis.com \
  artifactregistry.googleapis.com \
  iamcredentials.googleapis.com \
  iam.googleapis.com \
  config.googleapis.com \
  cloudresourcemanager.googleapis.com \
  --project=notes-app-gcp-504419
```

## 2. Create the deploy service account

```bash
gcloud iam service-accounts create notes-app-deploy \
  --project=notes-app-gcp-504419 \
  --display-name="notes-app CI/CD deploy" \
  --description="Used by GitHub Actions (via WIF) to deploy notes-app to Cloud Run."
```

## 3. Grant its project-level roles

```bash
DEPLOY_SA="notes-app-deploy@notes-app-gcp-504419.iam.gserviceaccount.com"

for ROLE in \
  roles/iam.serviceAccountUser \
  roles/artifactregistry.writer \
  roles/run.admin \
  roles/config.agent \
  roles/iam.workloadIdentityPoolAdmin \
; do
  gcloud projects add-iam-policy-binding notes-app-gcp-504419 \
    --member="serviceAccount:${DEPLOY_SA}" \
    --role="${ROLE}" \
    --condition=None
done
```

- `iam.serviceAccountUser`, `artifactregistry.writer`, `run.admin` — ongoing app-facing needs
  (deploying images, updating the Cloud Run service).
- `config.agent`, `iam.workloadIdentityPoolAdmin` — granted defensively during setup; not
  currently exercised since no Infrastructure Manager apply is used. Harmless to leave; revisit
  if the deploy SA's role list is ever tightened.

## 4. Create the Workload Identity Federation pool and provider

```bash
gcloud iam workload-identity-pools create github-actions \
  --project=notes-app-gcp-504419 \
  --location=global \
  --display-name="GitHub Actions" \
  --description="Identity pool for GitHub Actions OIDC tokens."

gcloud iam workload-identity-pools providers create-oidc github-actions \
  --project=notes-app-gcp-504419 \
  --location=global \
  --workload-identity-pool=github-actions \
  --display-name="GitHub Actions OIDC" \
  --issuer-uri="https://token.actions.githubusercontent.com" \
  --attribute-mapping="google.subject=assertion.sub,attribute.repository=assertion.repository,attribute.ref=assertion.ref" \
  --attribute-condition='assertion.repository == "bscheibe/notes-app"'
```

## 5. Allow GitHub Actions to impersonate the deploy SA

```bash
PROJECT_NUMBER=$(gcloud projects describe notes-app-gcp-504419 --format="value(projectNumber)")

gcloud iam service-accounts add-iam-policy-binding "${DEPLOY_SA}" \
  --project=notes-app-gcp-504419 \
  --role=roles/iam.workloadIdentityUser \
  --member="principalSet://iam.googleapis.com/projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/github-actions/attribute.repository/bscheibe/notes-app"
```

## 6. Create the Artifact Registry Docker repository

```bash
gcloud artifacts repositories create notes-app \
  --project=notes-app-gcp-504419 \
  --location=us-central1 \
  --repository-format=docker \
  --description="Docker images for the notes-app Cloud Run service."
```

## What's tracked where

| Resource | Managed by |
|---|---|
| Project API enablement | Manual (`gcloud services enable`, above) |
| `notes-app-deploy` service account | Manual (`gcloud iam service-accounts create`, above) |
| Deploy SA's project IAM roles | Manual (`gcloud projects add-iam-policy-binding`, above) |
| WIF pool/provider | Manual (`gcloud iam workload-identity-pools create/providers create-oidc`, above) |
| Deploy SA's WIF impersonation binding | Manual (`gcloud iam service-accounts add-iam-policy-binding`, above) |
| Artifact Registry repo | Manual (`gcloud artifacts repositories create`, above) |
| Cloud Run service | Terraform, [`infra/`](../infra) — applied manually by a human, never by CI, never via Infrastructure Manager |
| `run.admin` on the Cloud Run service, scoped to the deploy SA | Terraform, `infra/` |

There is no Terraform state for the manual steps above — they're a one-time setup, not an
ongoing-diff resource. If any of them need to change in the future, edit the underlying
`gcloud` command in this doc and re-run it by hand.

GitHub repo secrets that feed off these resources (`GCP_PROJECT_ID`, `GCP_REGION`,
`GCP_WORKLOAD_IDENTITY_PROVIDER`, `GCP_DEPLOY_SERVICE_ACCOUNT`) need updating to point at
`notes-app-gcp-504419` and the values created above before the deploy workflow will work.
