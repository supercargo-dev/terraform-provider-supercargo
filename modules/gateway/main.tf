resource "random_id" "suffix" {
  byte_length = 2
}

terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.0"
    }
  }
}

resource "google_project_service" "gateway_apis" {
  for_each = toset([
    "run.googleapis.com",
    "pubsub.googleapis.com",
    "cloudkms.googleapis.com",
    "datalineage.googleapis.com",
    "monitoring.googleapis.com",
    "logging.googleapis.com",
  ])

  project            = var.project_id
  service            = each.key
  disable_on_destroy = false
}

resource "time_sleep" "wait_for_gateway_apis" {
  depends_on      = [google_project_service.gateway_apis]
  create_duration = "30s"
}

resource "google_service_account" "gateway" {
  account_id   = "gateway-${var.product_id}-${random_id.suffix.hex}"
  display_name = "Gateway Service Account for ${var.product_id}"
  project      = var.project_id
  depends_on   = [time_sleep.wait_for_gateway_apis]
}

resource "google_cloud_run_v2_service" "gateway" {
  name     = "gateway-${var.product_id}-${random_id.suffix.hex}"
  location = var.region
  project  = var.project_id

  ingress = var.ingress_type

  custom_audiences = var.custom_audiences

  deletion_protection = false

  depends_on = [
    time_sleep.wait_for_gateway_apis,
    google_project_iam_member.gateway_logging,
    google_project_iam_member.gateway_monitoring,
    google_pubsub_topic_iam_member.gateway_publisher_clean,
    google_pubsub_topic_iam_member.gateway_publisher_dlq
  ]

  template {
    service_account = google_service_account.gateway.email
    containers {
      image = var.image
      env {
        name  = "GCP_PROJECT_ID"
        value = var.project_id
      }
      env {
        name  = "PRODUCT_ID"
        value = var.product_id
      }
      env {
        name  = "LOG_LEVEL"
        value = var.log_level
      }
      env {
        name  = "HUB_ADDRESS"
        value = var.hub_address
      }
      env {
        name  = "VAULT_ADDRESS"
        value = var.kms_address
      }
      env {
        name  = "CLEAN_TOPIC"
        value = google_pubsub_topic.clean.name
      }
      env {
        name  = "DLQ_TOPIC"
        value = google_pubsub_topic.dlq.name
      }
      env {
        name  = "MASTER_KEY_URI"
        value = "gcp-kms://${var.master_key_uri}"
      }
      env {
        name  = "HUB_IAP_CLIENT_ID"
        value = var.hub_iap_client_id
      }
      env {
        name  = "VAULT_IAP_CLIENT_ID"
        value = var.vault_iap_client_id
      }
      env {
        name  = "HUB_OIDC_AUDIENCE"
        value = var.hub_oidc_audience
      }
      env {
        name  = "VAULT_OIDC_AUDIENCE"
        value = var.vault_oidc_audience
      }
      env {
        name  = "GATEWAY_AUDIENCE"
        value = var.gateway_audience
      }
      env {
        name  = "AUTH_ENFORCE"
        value = var.auth_enforce ? "true" : "false"
      }

      env {
        name  = "FORCE_DEPLOY"
        value = var.force_deploy_trigger
      }
      resources {
        limits = {
          memory = var.container_memory
          cpu    = var.container_cpu
        }
      }
    }
  }
}

// Topics
resource "google_pubsub_topic" "raw" {
  name       = "raw-${var.product_id}-${random_id.suffix.hex}"
  project    = var.project_id
  depends_on = [google_project_service.gateway_apis]
}

resource "google_pubsub_topic" "clean" {
  name       = "clean-${var.product_id}-${random_id.suffix.hex}"
  project    = var.project_id
  depends_on = [google_project_service.gateway_apis]
}

resource "google_pubsub_topic" "dlq" {
  name       = "dlq-${var.product_id}-${random_id.suffix.hex}"
  project    = var.project_id
  depends_on = [google_project_service.gateway_apis]
}

resource "google_bigquery_table" "validated_data" {
  for_each   = var.contracts
  dataset_id = var.bigquery_dataset_id
  table_id   = "validated_${replace(each.key, "-", "_")}_${random_id.suffix.hex}"
  project    = var.project_id

  deletion_protection = false

  schema = each.value.schema
}

// IAM: Gateway needs to publish to Clean
resource "google_pubsub_topic_iam_member" "gateway_publisher_clean" {
  topic   = google_pubsub_topic.clean.name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.gateway.email}"
  project = var.project_id
}

// IAM: Gateway needs to publish to DLQ (if we do explicit dead-lettering)
resource "google_pubsub_topic_iam_member" "gateway_publisher_dlq" {
  topic   = google_pubsub_topic.dlq.name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.gateway.email}"
  project = var.project_id
}

resource "google_cloud_run_v2_service_iam_member" "authorized_invokers" {
  for_each = toset(var.authorized_invokers)
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.gateway.name
  role     = "roles/run.invoker"
  member   = each.value
}

resource "google_project_iam_member" "gateway_logging" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.gateway.email}"
}

resource "google_project_iam_member" "gateway_monitoring" {
  project = var.project_id
  role    = "roles/monitoring.metricWriter"
  member  = "serviceAccount:${google_service_account.gateway.email}"
}

resource "google_service_account_iam_member" "gateway_token_creator" {
  service_account_id = google_service_account.gateway.name
  role               = "roles/iam.serviceAccountOpenIdTokenCreator"
  member             = "serviceAccount:${google_service_account.gateway.email}"
}
