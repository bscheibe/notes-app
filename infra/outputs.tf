# WIF pool/provider, the deploy service account, and the Artifact Registry
# repo are all created via `gcloud`, not Terraform (see
# docs/CLOUD_RUN_CICD_BOOTSTRAP.md), so there's nothing to output for them
# here - this root only manages the Cloud Run service itself.

output "cloud_run_service_url" {
  description = "URL of the deployed Cloud Run service."
  value       = google_cloud_run_v2_service.notes_app.uri
}
