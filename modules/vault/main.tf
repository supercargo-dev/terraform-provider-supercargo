resource "google_project_service" "vault_apis" {
  for_each = toset([
    "cloudkms.googleapis.com",
    "secretmanager.googleapis.com",
    "pubsub.googleapis.com",
    "iap.googleapis.com",
    "monitoring.googleapis.com",
    "logging.googleapis.com",
  ])

  project            = var.project_id
  service            = each.key
  disable_on_destroy = false
}

resource "time_sleep" "wait_for_vault_apis" {
  depends_on      = [google_project_service.vault_apis]
  create_duration = "30s"
}

resource "random_id" "suffix" {
  byte_length = 2
}

resource "google_service_account" "vault_sa" {
  account_id   = "supercargo-vault-${random_id.suffix.hex}"
  display_name = "Supercargo Vault Service Account"
  project      = var.project_id
  depends_on   = [time_sleep.wait_for_vault_apis]
}

resource "google_secret_manager_secret" "vault_master_key" {
  project   = var.project_id
  secret_id = "supercargo-vault-master-key-${random_id.suffix.hex}"
  replication {
    auto {}
  }
  depends_on = [google_project_service.vault_apis]
}

resource "random_id" "vault_master_key" {
  byte_length = 32
}

resource "google_secret_manager_secret_version" "vault_master_key_data" {
  secret      = google_secret_manager_secret.vault_master_key.id
  secret_data = random_id.vault_master_key.b64_std
}

resource "google_secret_manager_secret_iam_member" "vault_sa_secret_accessor" {
  project   = var.project_id
  secret_id = google_secret_manager_secret.vault_master_key.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.vault_sa.email}"
}

resource "google_secret_manager_secret" "global_pepper" {
  project   = var.project_id
  secret_id = "supercargo-vault-global-pepper-${random_id.suffix.hex}"
  replication {
    auto {}
  }
  depends_on = [google_project_service.vault_apis]
}

resource "random_password" "global_pepper" {
  length  = 32
  special = false
}

resource "google_secret_manager_secret_version" "global_pepper_data" {
  secret      = google_secret_manager_secret.global_pepper.id
  secret_data = random_password.global_pepper.result
}

resource "google_secret_manager_secret_iam_member" "vault_sa_pepper_accessor" {
  project   = var.project_id
  secret_id = google_secret_manager_secret.global_pepper.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.vault_sa.email}"
}

resource "google_kms_key_ring" "keyring" {
  name     = var.keyring_name
  location = var.region
  project  = var.project_id

  depends_on = [google_project_service.vault_apis]
}

resource "google_kms_crypto_key" "master_key" {
  name            = var.crypto_key_name
  key_ring        = google_kms_key_ring.keyring.id
  rotation_period = var.key_rotation_period

  lifecycle {
    prevent_destroy = true
  }
}

resource "google_kms_crypto_key_iam_member" "vault_sa_binding" {
  crypto_key_id = google_kms_crypto_key.master_key.id
  role          = "roles/cloudkms.cryptoKeyEncrypterDecrypter"
  member        = "serviceAccount:${google_service_account.vault_sa.email}"
}

resource "google_kms_crypto_key_iam_member" "gateway_sa_binding" {
  crypto_key_id = google_kms_crypto_key.master_key.id
  role          = "roles/cloudkms.cryptoKeyEncrypterDecrypter"
  member        = "serviceAccount:${var.gateway_service_account_email}"
}

resource "google_project_iam_member" "vault_firestore_user" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.vault_sa.email}"
}

resource "google_project_iam_member" "vault_logging" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.vault_sa.email}"
}

resource "google_project_iam_member" "vault_monitoring" {
  project = var.project_id
  role    = "roles/monitoring.metricWriter"
  member  = "serviceAccount:${google_service_account.vault_sa.email}"
}

resource "google_service_account_iam_member" "vault_token_creator" {
  service_account_id = google_service_account.vault_sa.name
  role               = "roles/iam.serviceAccountOpenIdTokenCreator"
  member             = "serviceAccount:${google_service_account.vault_sa.email}"
}

resource "google_pubsub_topic_iam_member" "vault_pubsub_publisher" {
  project = var.project_id
  topic   = google_pubsub_topic.key_created.name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.vault_sa.email}"
}

resource "google_pubsub_topic" "key_created" {
  name       = "com.supercargo.security.key_created.v1-${random_id.suffix.hex}"
  project    = var.project_id
  depends_on = [google_project_service.vault_apis]
}

resource "google_cloud_run_v2_service" "vault" {
  provider = google-beta
  name     = "${var.service_name}-${random_id.suffix.hex}"
  location = var.region
  project  = var.project_id

  launch_stage = "BETA"

  # Production logic: Internal-only ingress (or controlled via variable)
  ingress = var.ingress_type

  # Enable Direct IAP (Preview feature)
  iap_enabled = var.iap_enabled

  custom_audiences = var.custom_audiences

  deletion_protection = false

  depends_on = [
    time_sleep.wait_for_vault_apis,
    google_secret_manager_secret_iam_member.vault_sa_secret_accessor,
    google_secret_manager_secret_iam_member.vault_sa_pepper_accessor
  ]

  template {
    service_account = google_service_account.vault_sa.email

    containers {
      image = var.image

      env {
        name  = "GCP_PROJECT_ID"
        value = var.project_id
      }

      env {
        name  = "GLOBAL_PEPPER_SECRET_ID"
        value = google_secret_manager_secret.global_pepper.secret_id
      }

      env {
        name  = "MASTER_KEY_ID"
        value = google_kms_crypto_key.master_key.id
      }

      env {
        name  = "KEY_CREATED_TOPIC_ID"
        value = google_pubsub_topic.key_created.name
      }

      env {
        name  = "OIDC_AUDIENCE"
        value = var.oidc_audience
      }

      env {
        name  = "AUTH_ENFORCE"
        value = var.auth_enforce ? "true" : "false"
      }

      env {
        name  = "FORCE_DEPLOY"
        value = var.force_deploy_trigger
      }

      env {
        name = "VAULT_MASTER_KEY"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.vault_master_key.secret_id
            version = "latest"
          }
        }
      }

      ports {
        container_port = 8080
        name           = "h2c"
      }
    }
  }

  lifecycle {
    ignore_changes = [
      template[0].containers[0].image,
      client,
      client_version
    ]
  }
}

resource "google_cloud_run_v2_service_iam_member" "gateway_invoker" {
  name     = google_cloud_run_v2_service.vault.name
  location = google_cloud_run_v2_service.vault.location
  project  = google_cloud_run_v2_service.vault.project
  role     = "roles/run.invoker"
  member   = "serviceAccount:${var.gateway_service_account_email}"
}

// Explicitly create the IAP Service Agent to avoid "member does not exist" errors
resource "google_project_service_identity" "iap_sa" {
  count    = var.iap_enabled ? 1 : 0
  provider = google-beta
  project  = var.project_id
  service  = "iap.googleapis.com"

  depends_on = [google_project_service.vault_apis]
}

# Grant the IAP Service Agent permission to invoke the Cloud Run service
resource "google_cloud_run_v2_service_iam_member" "iap_invoker" {
  count    = var.iap_enabled ? 1 : 0
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.vault.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_project_service_identity.iap_sa[0].email}"
}

resource "google_cloud_run_v2_service_iam_member" "additional_invokers" {
  for_each = toset(var.authorized_invokers)
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.vault.name
  role     = "roles/run.invoker"
  member   = each.value
}

# Grant authorized users access via IAP
resource "google_iap_web_cloud_run_service_iam_member" "iap_accessor" {
  provider               = google-beta
  for_each               = var.iap_enabled ? toset(var.iap_allowed_members) : []
  project                = var.project_id
  cloud_run_service_name = google_cloud_run_v2_service.vault.name
  location               = var.region
  role                   = "roles/iap.httpsResourceAccessor"
  member                 = each.value
}
