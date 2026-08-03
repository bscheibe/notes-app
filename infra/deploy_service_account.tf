# The deploy service account itself, its project-level IAM roles, the WIF
# pool/provider, and the Artifact Registry repo are all created manually via
# `gcloud`, one time, not by Terraform - see docs/CLOUD_RUN_CICD_BOOTSTRAP.md.
# Terraform's scope here is just the application resource (the Cloud Run
# service) plus IAM on THAT resource, referencing the deploy SA as a data
# source. This isn't only about the self-management IAM problem (a SA
# granting itself project IAM needs resourcemanager.projects.setIamPolicy,
# which it structurally shouldn't have) - it's a simpler split now: CI/CD
# plumbing is one-time setup done by hand, the app itself is what Terraform
# manages, applied manually (no Infrastructure Manager, no CI-triggered
# apply).
data "google_service_account" "deploy" {
  project    = var.project_id
  account_id = "notes-app-deploy"
}

# roles/run.admin is granted scoped to the notes-app service itself (see
# cloud_run.tf), not at the project level, to avoid handing this account
# admin rights over every Cloud Run service in the project.
resource "google_cloud_run_v2_service_iam_member" "deploy_run_admin" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.notes_app.name
  role     = "roles/run.admin"
  member   = "serviceAccount:${data.google_service_account.deploy.email}"
}
