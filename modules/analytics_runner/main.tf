locals {
  runner_sa_email    = "${var.service_account_id != null && var.service_account_id != "" ? var.service_account_id : "${var.job_name}-sa"}@${var.project_id}.iam.gserviceaccount.com"
  scheduler_sa_email = "${var.scheduler_service_account_id != null && var.scheduler_service_account_id != "" ? var.scheduler_service_account_id : "${var.job_name}-sched-sa"}@${var.project_id}.iam.gserviceaccount.com"
}

resource "google_service_account" "runner_sa" {
  account_id   = var.service_account_id != null && var.service_account_id != "" ? var.service_account_id : "${var.job_name}-sa"
  display_name = "Analytics Runner Service Account"
  project      = var.project_id
}

resource "google_service_account" "scheduler_sa" {
  account_id   = var.scheduler_service_account_id != null && var.scheduler_service_account_id != "" ? var.scheduler_service_account_id : "${var.job_name}-sched-sa"
  display_name = "Analytics Runner Cloud Scheduler Service Account"
  project      = var.project_id
}

resource "google_cloud_run_v2_job" "analytics_runner" {
  name                = var.job_name
  location            = var.region
  project             = var.project_id
  deletion_protection = var.deletion_protection

  template {
    template {
      service_account = local.runner_sa_email
      max_retries     = var.max_retries
      timeout         = var.timeout

      containers {
        image = var.container_image
        args  = ["analytics", "run", "--engine", var.engine, "--dialect", var.dialect]

        env {
          name  = "GOOGLE_CLOUD_PROJECT"
          value = var.project_id
        }

        env {
          name  = "SUPERCARGO_DWH_REGION"
          value = var.region
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

  depends_on = [
    google_service_account.runner_sa,
  ]
}

resource "google_cloud_scheduler_job" "runner_schedule" {
  name        = "${var.job_name}-schedule"
  description = "Scheduled trigger for Analytics Runner Cloud Run Job"
  project     = var.project_id
  region      = var.region
  schedule    = var.schedule
  time_zone   = var.time_zone
  paused      = var.paused

  http_target {
    http_method = "POST"
    uri         = "https://${var.region}-run.googleapis.com/v2/projects/${var.project_id}/locations/${var.region}/jobs/${google_cloud_run_v2_job.analytics_runner.name}:run"

    oauth_token {
      service_account_email = local.scheduler_sa_email
      scope                 = "https://www.googleapis.com/auth/cloud-platform"
    }
  }

  depends_on = [
    google_cloud_run_v2_job_iam_member.scheduler_invoker,
  ]
}

resource "google_cloud_run_v2_job_iam_member" "scheduler_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_job.analytics_runner.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${local.scheduler_sa_email}"

  depends_on = [
    google_service_account.scheduler_sa,
    google_cloud_run_v2_job.analytics_runner,
  ]
}

resource "google_project_iam_member" "job_user" {
  project = var.project_id
  role    = "roles/bigquery.jobUser"
  member  = "serviceAccount:${local.runner_sa_email}"

  depends_on = [
    google_service_account.runner_sa,
  ]
}

resource "google_bigquery_dataset_iam_member" "target_data_editor" {
  for_each   = toset(var.target_datasets)
  project    = var.project_id
  dataset_id = each.value
  role       = "roles/bigquery.dataEditor"
  member     = "serviceAccount:${local.runner_sa_email}"

  depends_on = [
    google_service_account.runner_sa,
  ]
}

resource "google_bigquery_dataset_iam_member" "input_data_viewer" {
  for_each   = toset(var.input_datasets)
  project    = var.project_id
  dataset_id = each.value
  role       = "roles/bigquery.dataViewer"
  member     = "serviceAccount:${local.runner_sa_email}"

  depends_on = [
    google_service_account.runner_sa,
  ]
}
