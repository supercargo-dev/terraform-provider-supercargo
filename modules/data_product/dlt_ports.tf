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
          try(each.value.contract_urn, try(each.value.contract, each.key)),
          "--product-urn",
          data.supercargo_data_product.this.urn,
          "--destination",
          "bigquery",
          "--dataset",
          local.dlt_bigquery_dataset,
          "--pipeline-name",
          "dlt-${local.product_id}-${replace(each.key, "_", "-")}",
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
