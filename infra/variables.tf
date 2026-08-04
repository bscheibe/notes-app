variable "project_id" {
  description = "GCP project ID to deploy into."
  type        = string
  default     = "notes-app-gcp-504419"
}

variable "region" {
  description = "GCP region for Cloud Run."
  type        = string
  default     = "us-central1"
}

variable "service_name" {
  description = "Name of the Cloud Run service."
  type        = string
  default     = "notes-app"
}

variable "artifact_registry_repository_id" {
  description = "Name of the Artifact Registry Docker repository (created via gcloud, not Terraform - see docs/CLOUD_RUN_CICD_BOOTSTRAP.md). Used here only to reference the image path."
  type        = string
  default     = "notes-app"
}
