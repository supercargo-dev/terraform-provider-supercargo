locals {
  sa_email           = "${var.service_account_id != null && var.service_account_id != "" ? var.service_account_id : "analytics-github-sync-sa"}@${var.project_id}.iam.gserviceaccount.com"
  scheduler_sa_email = "${var.scheduler_service_account_id != null && var.scheduler_service_account_id != "" ? var.scheduler_service_account_id : "analytics-github-sync-sched-sa"}@${var.project_id}.iam.gserviceaccount.com"

  contracts = {
    repositories  = { job_suffix = "repositories", resource = "labeled_repositories", schedule = "0 1 * * *" }
    issues        = { job_suffix = "issues", resource = "issues", schedule = "0 2 * * *" }
    pulls         = { job_suffix = "pulls", resource = "pulls", schedule = "0 3 * * *" }
    workflow_runs = { job_suffix = "workflow-runs", resource = "workflow_runs", schedule = "0 4 * * *" }
    billing       = { job_suffix = "billing", resource = "org_metered_usage", schedule = "0 5 * * *" }
  }
}

resource "google_service_account" "sync_sa" {
  account_id   = var.service_account_id != null && var.service_account_id != "" ? var.service_account_id : "analytics-github-sync-sa"
  display_name = "Analytics GitHub Sync Service Account"
  project      = var.project_id
}

resource "google_service_account" "scheduler_sa" {
  account_id   = var.scheduler_service_account_id != null && var.scheduler_service_account_id != "" ? var.scheduler_service_account_id : "analytics-github-sync-sched-sa"
  display_name = "Analytics GitHub Sync Cloud Scheduler Service Account"
  project      = var.project_id
}

resource "google_cloud_run_v2_job" "sync_jobs" {
  for_each            = local.contracts
  name                = "${var.job_name}-${each.value.job_suffix}"
  location            = var.region
  project             = var.project_id
  deletion_protection = var.deletion_protection

  template {
    template {
      service_account = local.sa_email
      max_retries     = var.max_retries
      timeout         = var.timeout

      containers {
        image = var.container_image
        args = [
          "--destination", "bigquery",
          "--dataset", var.dataset,
          "--org", var.org,
          "--topic", var.topic,
          "--resources", each.value.resource
        ]

        env {
          name  = "SUPERCARGO_VAULT_URL"
          value = var.vault_url
        }
        env {
          name  = "SUPERCARGO_VAULT_AUDIENCE"
          value = var.vault_audience
        }
        env {
          name  = "SUPERCARGO_HUB_URL"
          value = var.hub_url
        }
        env {
          name  = "SUPERCARGO_HUB_AUDIENCE"
          value = var.hub_audience
        }
        env {
          name  = "GOOGLE_CLOUD_PROJECT"
          value = var.project_id
        }
        env {
          name  = "DESTINATION__BIGQUERY__LOCATION"
          value = var.region
        }
        env {
          name = "GITHUB_TOKEN"
          value_source {
            secret_key_ref {
              secret  = var.github_token_secret_id
              version = "latest"
            }
          }
        }
        env {
          name = "SUPERCARGO_CREDENTIALS_SECRET"
          value_source {
            secret_key_ref {
              secret  = var.github_token_secret_id
              version = "latest"
            }
          }
        }

        resources {
          limits = {
            cpu    = var.cpu_limit
            memory = var.memory_limit
          }
        }
      }
    }
  }

  lifecycle {
    ignore_changes = [
      template[0].template[0].containers[0].image,
      client,
      client_version,
    ]
  }

  depends_on = [
    google_service_account.sync_sa,
  ]
}

resource "google_cloud_run_v2_job_iam_member" "scheduler_invoker" {
  for_each = local.contracts
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_job.sync_jobs[each.key].name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${local.scheduler_sa_email}"

  depends_on = [
    google_service_account.scheduler_sa,
    google_cloud_run_v2_job.sync_jobs,
  ]
}

resource "google_cloud_scheduler_job" "sync_schedules" {
  for_each         = local.contracts
  name             = "${var.job_name}-${each.value.job_suffix}-schedule"
  description      = "Scheduled trigger for Analytics GitHub Sync (${each.key}) Cloud Run Job"
  schedule         = try(var.schedules[each.key], each.value.schedule)
  time_zone        = var.time_zone
  paused           = var.paused
  region           = var.region
  project          = var.project_id
  attempt_deadline = var.scheduler_attempt_deadline

  http_target {
    http_method = "POST"
    uri         = "https://${var.region}-run.googleapis.com/v2/projects/${var.project_id}/locations/${var.region}/jobs/${google_cloud_run_v2_job.sync_jobs[each.key].name}:run"

    oauth_token {
      service_account_email = local.scheduler_sa_email
      scope                 = "https://www.googleapis.com/auth/cloud-platform"
    }
  }

  depends_on = [
    google_cloud_run_v2_job_iam_member.scheduler_invoker,
  ]
}

moved {
  from = google_cloud_run_v2_job.sync_job
  to   = google_cloud_run_v2_job.sync_jobs["repositories"]
}

moved {
  from = google_cloud_scheduler_job.sync_schedule
  to   = google_cloud_scheduler_job.sync_schedules["repositories"]
}

resource "google_bigquery_dataset_iam_member" "sync_job_bq_editor" {
  project    = var.project_id
  dataset_id = var.dataset
  role       = "roles/bigquery.dataEditor"
  member     = "serviceAccount:${local.sa_email}"

  depends_on = [
    google_service_account.sync_sa,
  ]
}

resource "google_project_iam_member" "sync_job_bq_user" {
  project = var.project_id
  role    = "roles/bigquery.jobUser"
  member  = "serviceAccount:${local.sa_email}"

  depends_on = [
    google_service_account.sync_sa,
  ]
}

resource "google_secret_manager_secret_iam_member" "github_token_accessor" {
  project   = var.project_id
  secret_id = var.github_token_secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${local.sa_email}"

  depends_on = [
    google_service_account.sync_sa,
  ]
}
