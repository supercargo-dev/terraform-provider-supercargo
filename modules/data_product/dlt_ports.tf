locals {
  # AC-1: Filter DLT input ports directly from manifest input ports without redefining raw_input_ports
  dlt_input_ports = {
    for port in local.manifest_input_ports : port.name => port
    if(can(port.type) && port.type == "dlt") || (can(port.source) && startswith(port.source, "dlt://"))
  }

  # AC-1: BigQuery dataset fallback
  dlt_bigquery_dataset = var.bigquery_dataset_id != "" ? var.bigquery_dataset_id : replace(local.product_id, "-", "_")

  # AC-2: Collision-resistant Runner Service Account ID and Email
  candidate_runner_sa = var.dlt_runner_service_account_id != "" ? var.dlt_runner_service_account_id : "dlt-r-${local.product_id}${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  dlt_runner_sa_id    = length(local.candidate_runner_sa) <= 30 ? local.candidate_runner_sa : "${substr(regexreplace(substr(local.candidate_runner_sa, 0, 23), "-+$", ""), 0, 23)}-${substr(md5(local.candidate_runner_sa), 0, 6)}"
  dlt_runner_sa_email = "${local.dlt_runner_sa_id}@${var.project_id}.iam.gserviceaccount.com"

  # AC-2: Collision-resistant Scheduler Service Account ID and Email
  candidate_scheduler_sa = var.dlt_scheduler_service_account_id != "" ? var.dlt_scheduler_service_account_id : "dlt-s-${local.product_id}${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  dlt_scheduler_sa_id    = length(local.candidate_scheduler_sa) <= 30 ? local.candidate_scheduler_sa : "${substr(regexreplace(substr(local.candidate_scheduler_sa, 0, 23), "-+$", ""), 0, 23)}-${substr(md5(local.candidate_scheduler_sa), 0, 6)}"
  dlt_scheduler_sa_email = "${local.dlt_scheduler_sa_id}@${var.project_id}.iam.gserviceaccount.com"
}

# AC-2: Dedicated Service Account for DLT Runner
resource "google_service_account" "dlt_runner" {
  count        = length(local.dlt_input_ports) > 0 ? 1 : 0
  account_id   = local.dlt_runner_sa_id
  display_name = "DLT Ingestion Runner Service Account for ${local.product_id}"
  project      = var.project_id
}

# AC-3 & AC-4: Cloud Run v2 Jobs for DLT Ingestion
resource "google_cloud_run_v2_job" "dlt_jobs" {
  for_each            = local.dlt_input_ports
  name                = "dlt-${local.product_id}-${replace(each.key, "_", "-")}${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  location            = var.region
  project             = var.project_id
  deletion_protection = var.dlt_deletion_protection

  template {
    template {
      service_account = local.dlt_runner_sa_email
      max_retries     = var.dlt_max_retries
      timeout         = var.dlt_timeout

      containers {
        image = var.dlt_runner_image
        args = compact(flatten([
          "--source",
          each.value.source,
          "--contract-urn",
          try(each.value.contract_urn, try(each.value.contract.urn, try(each.value.contract, each.key))),
          "--product-urn",
          data.supercargo_data_product.this.urn,
          "--destination",
          "bigquery",
          "--dataset",
          local.dlt_bigquery_dataset,
          "--pipeline-name",
          "dlt_${replace(local.product_id, "-", "_")}_${replace(each.key, "-", "_")}${var.name_suffix != "" ? "_${replace(var.name_suffix, "-", "_")}" : ""}",
          can(regex("(sql_database|filesystem)", each.value.source)) ? ["--table", try(each.value.table, each.key)] : [],
          try(each.value.cursor_field, "") != "" ? ["--cursor-field", each.value.cursor_field] : [],
          try(each.value.primary_key, "") != "" ? ["--primary-key", each.value.primary_key] : [],
          try(each.value.write_disposition, "") != "" ? ["--write-disposition", each.value.write_disposition] : [],
          try(each.value.cold_start_window, "") != "" ? ["--cold-start-window", each.value.cold_start_window] : [],
          try(each.value.entrypoint, "") != "" ? ["--entrypoint", each.value.entrypoint] : [],
        ]))

        env {
          name  = "SUPERCARGO_HUB_URL"
          value = var.hub_address
        }
        env {
          name  = "SUPERCARGO_HUB_AUDIENCE"
          value = var.hub_oidc_audience
        }
        env {
          name  = "SUPERCARGO_VAULT_URL"
          value = var.kms_address
        }
        env {
          name  = "SUPERCARGO_VAULT_AUDIENCE"
          value = var.vault_oidc_audience
        }
        env {
          name  = "GOOGLE_CLOUD_PROJECT"
          value = var.project_id
        }
        env {
          name  = "DESTINATION__BIGQUERY__LOCATION"
          value = var.region
        }

        dynamic "env" {
          for_each = try(each.value.credentials_secret, "") != "" ? [each.value.credentials_secret] : []
          content {
            name = "SUPERCARGO_DB_CREDENTIALS"
            value_source {
              secret_key_ref {
                secret  = env.value
                version = "latest"
              }
            }
          }
        }

        dynamic "env" {
          for_each = try(each.value.credentials_secret, "") != "" ? [each.value.credentials_secret] : []
          content {
            name = "SUPERCARGO_CREDENTIALS_SECRET"
            value_source {
              secret_key_ref {
                secret  = env.value
                version = "latest"
              }
            }
          }
        }

        resources {
          limits = {
            cpu    = var.dlt_cpu
            memory = var.dlt_memory
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
    google_service_account.dlt_runner,
  ]
}

# AC-5: Dedicated Service Account for Cloud Scheduler
resource "google_service_account" "dlt_scheduler" {
  count        = length(local.dlt_input_ports) > 0 ? 1 : 0
  account_id   = local.dlt_scheduler_sa_id
  display_name = "DLT Cloud Scheduler Service Account for ${local.product_id}"
  project      = var.project_id
}

# AC-5: IAM Role Run Invoker for Cloud Scheduler SA
resource "google_cloud_run_v2_job_iam_member" "dlt_scheduler_invoker" {
  for_each = local.dlt_input_ports
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_job.dlt_jobs[each.key].name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${local.dlt_scheduler_sa_email}"

  depends_on = [
    google_service_account.dlt_scheduler,
    google_cloud_run_v2_job.dlt_jobs,
  ]
}

# AC-5: Cloud Scheduler Jobs to Trigger DLT Ingestion
resource "google_cloud_scheduler_job" "dlt_schedules" {
  for_each         = local.dlt_input_ports
  name             = "dlt-sched-${local.product_id}-${replace(each.key, "_", "-")}${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  schedule         = try(each.value.schedule, "") != "" ? each.value.schedule : "0 2 * * *"
  time_zone        = try(each.value.time_zone, "") != "" ? each.value.time_zone : var.dlt_time_zone
  attempt_deadline = var.dlt_scheduler_attempt_deadline
  region           = var.region
  project          = var.project_id

  http_target {
    http_method = "POST"
    uri         = "https://${var.region}-run.googleapis.com/v2/projects/${var.project_id}/locations/${var.region}/jobs/${google_cloud_run_v2_job.dlt_jobs[each.key].name}:run"

    oauth_token {
      service_account_email = local.dlt_scheduler_sa_email
      scope                 = "https://www.googleapis.com/auth/cloud-platform"
    }
  }

  depends_on = [
    google_cloud_run_v2_job_iam_member.dlt_scheduler_invoker,
    google_cloud_run_v2_job.dlt_jobs,
  ]
}

# AC-6: BigQuery Data Editor IAM Binding for DLT Runner SA
resource "google_project_iam_member" "dlt_runner_bq_editor" {
  count   = length(local.dlt_input_ports) > 0 ? 1 : 0
  project = var.project_id
  role    = "roles/bigquery.dataEditor"
  member  = "serviceAccount:${local.dlt_runner_sa_email}"

  depends_on = [
    google_service_account.dlt_runner,
  ]
}

# AC-6: BigQuery Job User IAM Binding for DLT Runner SA
resource "google_project_iam_member" "dlt_runner_bq_user" {
  count   = length(local.dlt_input_ports) > 0 ? 1 : 0
  project = var.project_id
  role    = "roles/bigquery.jobUser"
  member  = "serviceAccount:${local.dlt_runner_sa_email}"

  depends_on = [
    google_service_account.dlt_runner,
  ]
}

# AC-6: Secret Manager Secret Accessor IAM Binding for DLT Runner SA
resource "google_secret_manager_secret_iam_member" "dlt_runner_secret_accessor" {
  for_each = toset([
    for k, v in local.dlt_input_ports : v.credentials_secret
    if try(v.credentials_secret, "") != ""
  ])
  project   = var.project_id
  secret_id = startswith(each.value, "projects/") ? element(split("/", each.value), length(split("/", each.value)) - 1) : each.value
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${local.dlt_runner_sa_email}"

  depends_on = [
    google_service_account.dlt_runner,
  ]
}

