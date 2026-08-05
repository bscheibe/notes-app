resource "google_cloud_run_v2_service" "notes_app" {
  project  = var.project_id
  name     = var.service_name
  location = var.region
  ingress  = "INGRESS_TRAFFIC_ALL"

  template {
    # No need to stay warm - not serving real traffic yet. App-layer Firebase
    # token verification (once that migration lands) is the real access gate,
    # not network isolation - see docs/CLOUD_RUN_DEPLOYMENT_PLAN.md.
    scaling {
      min_instance_count = 0
      # Capped low deliberately: the real defense against a cost blowout from
      # a traffic spike or abuse against this public, auth-gated endpoint.
      # Once capped, excess requests during a surge get 429s instead of
      # unbounded scaling and billing.
      max_instance_count = 3
    }

    # Conservative per-instance concurrency and a short request timeout,
    # paired with the max-instance cap above, per task 9 of the deployment
    # plan.
    max_instance_request_concurrency = 10
    timeout                          = "30s"

    containers {
      # Placeholder image for the initial `terraform apply` - the deploy
      # workflow (see .github/workflows/deploy.yml) updates this to the
      # actual built image on every release. Terraform won't fight that
      # update because `image` is ignored post-create (see lifecycle block).
      image = "us-docker.pkg.dev/cloudrun/container/hello"
    }

    service_account = data.google_service_account.deploy.email
  }

  lifecycle {
    ignore_changes = [
      template[0].containers[0].image,
    ]
  }

  # run.googleapis.com is enabled manually via `gcloud` before this root
  # ever runs (see docs/CLOUD_RUN_CICD_BOOTSTRAP.md) - no depends_on needed
  # here.
}

# Intentionally NOT publicly invokable yet, even though the deployment plan
# (docs/CLOUD_RUN_DEPLOYMENT_PLAN.md question 2) calls for `allUsers` once
# Firebase auth is the real access gate. Until that migration lands, the
# service runs a placeholder image with no auth check at all - leaving it
# public would mean an unauthenticated endpoint reachable by anyone. Standby
# state: re-add a `google_cloud_run_v2_service_iam_member` with
# role = "roles/run.invoker", member = "allUsers" once the Firebase
# middleware is actually deployed.
