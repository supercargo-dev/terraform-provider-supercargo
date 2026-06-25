resource "google_project_service" "hub_apis" {
  for_each = toset([
    "run.googleapis.com",
    "firestore.googleapis.com",
    "datastore.googleapis.com",
    "iam.googleapis.com",
    "iap.googleapis.com", // Added IAP API
    "iamcredentials.googleapis.com",
    "eventarc.googleapis.com",
    "datalineage.googleapis.com",
    "dataplex.googleapis.com", // Added Dataplex API
    "monitoring.googleapis.com",
    "logging.googleapis.com",
  ])

  project            = var.project_id
  service            = each.key
  disable_on_destroy = false
}

// Grant Hub access to create lineage
resource "google_project_iam_member" "hub_datalineage_producer" {
  project    = var.project_id
  role       = "roles/datalineage.producer"
  member     = "serviceAccount:${google_service_account.hub_runtime.email}"
  depends_on = [google_project_service.hub_apis]
}

// Grant Hub access to manage Dataplex Catalog and Data Products
resource "google_project_iam_member" "hub_dataplex_catalog_admin" {
  project    = var.project_id
  role       = "roles/dataplex.catalogAdmin"
  member     = "serviceAccount:${google_service_account.hub_runtime.email}"
  depends_on = [google_project_service.hub_apis]
}

resource "google_project_iam_member" "hub_dataplex_data_products_admin" {
  project    = var.project_id
  role       = "roles/dataplex.dataProductsAdmin"
  member     = "serviceAccount:${google_service_account.hub_runtime.email}"
  depends_on = [google_project_service.hub_apis]
}

// Explicitly create the IAP Service Agent to avoid "member does not exist" errors
resource "google_project_service_identity" "iap_sa" {
  count    = var.iap_enabled ? 1 : 0
  provider = google-beta
  project  = var.project_id
  service  = "iap.googleapis.com"

  depends_on = [google_project_service.hub_apis]
}

resource "google_cloud_run_v2_service_iam_member" "iap_invoker" {
  count    = var.iap_enabled ? 1 : 0
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.hub.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_project_service_identity.iap_sa[0].email}"
}

# Grant authorized users access via IAP
resource "google_iap_web_cloud_run_service_iam_member" "iap_accessor" {
  provider               = google-beta
  for_each               = var.iap_enabled ? toset(var.iap_allowed_members) : []
  project                = var.project_id
  cloud_run_service_name = google_cloud_run_v2_service.hub.name
  location               = var.region
  role                   = "roles/iap.httpsResourceAccessor"
  member                 = each.value
}

# Grant Gateway Service Account access to invoke Hub (IAM auth)
resource "google_cloud_run_v2_service_iam_member" "gateway_invoker" {
  count    = var.gateway_service_account_email != "" ? 1 : 0
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.hub.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${var.gateway_service_account_email}"
}

resource "google_firestore_database" "database" {
  project     = var.project_id
  name        = "(default)"
  location_id = var.region
  type        = "FIRESTORE_NATIVE"

  depends_on = [google_project_service.hub_apis]
}

resource "google_firestore_index" "outbox_relay_index" {
  project    = var.project_id
  collection = "outbox"

  fields {
    field_path = "action"
    order      = "ASCENDING"
  }

  fields {
    field_path = "created_at"
    order      = "ASCENDING"
  }

  fields {
    field_path = "__name__"
    order      = "ASCENDING"
  }

  depends_on = [google_firestore_database.database]
}

resource "google_firestore_index" "audit_runs_index" {
  project    = var.project_id
  collection = "audit_runs"

  fields {
    field_path = "product_urn"
    order      = "ASCENDING"
  }

  fields {
    field_path = "timestamp"
    order      = "DESCENDING"
  }

  fields {
    field_path = "__name__"
    order      = "DESCENDING"
  }

  depends_on = [google_firestore_database.database]
}

resource "google_firestore_field" "dataplex_cache_ttl" {
  project    = var.project_id
  collection = "dataplex_cache"
  field      = "expiresAt"

  ttl_config {}

  depends_on = [google_firestore_database.database]
}

resource "google_firestore_field" "mappings_dedupe_ttl" {
  project    = var.project_id
  collection = "mappings_dedupe"
  field      = "expires_at"

  ttl_config {}

  depends_on = [google_firestore_database.database]
}

resource "google_firestore_index" "dataplex_cache_index" {
  project    = var.project_id
  collection = "dataplex_cache"

  fields {
    field_path = "method"
    order      = "ASCENDING"
  }

  fields {
    field_path = "target"
    order      = "ASCENDING"
  }

  fields {
    field_path = "project"
    order      = "ASCENDING"
  }

  fields {
    field_path = "location"
    order      = "ASCENDING"
  }

  fields {
    field_path = "isSource"
    order      = "ASCENDING"
  }

  depends_on = [google_firestore_database.database]
}

resource "google_firestore_index" "field_edges_from_index" {
  project    = var.project_id
  collection = "field_edges"

  fields {
    field_path = "from_field_urn"
    order      = "ASCENDING"
  }

  fields {
    field_path = "type"
    order      = "ASCENDING"
  }

  fields {
    field_path = "__name__"
    order      = "ASCENDING"
  }

  depends_on = [google_firestore_database.database]
}

resource "google_firestore_index" "field_edges_to_index" {
  project    = var.project_id
  collection = "field_edges"

  fields {
    field_path = "to_field_urn"
    order      = "ASCENDING"
  }

  fields {
    field_path = "type"
    order      = "ASCENDING"
  }

  fields {
    field_path = "__name__"
    order      = "ASCENDING"
  }

  depends_on = [google_firestore_database.database]
}

resource "google_firestore_index" "pii_masks_index" {
  project    = var.project_id
  collection = "pii_masks"

  fields {
    field_path = "kind"
    order      = "ASCENDING"
  }

  fields {
    field_path = "algorithm"
    order      = "ASCENDING"
  }

  fields {
    field_path = "__name__"
    order      = "ASCENDING"
  }

  depends_on = [google_firestore_database.database]
}

resource "time_sleep" "wait_for_apis" {
  depends_on = [google_firestore_database.database]

  create_duration = "60s"
}

resource "google_service_account" "hub_runtime" {
  project      = var.project_id
  account_id   = "hub-runtime"
  display_name = "Hub Runtime Service Account"
  depends_on   = [time_sleep.wait_for_apis]
}

resource "google_project_iam_member" "hub_firestore_user" {
  project    = var.firestore_project_id
  role       = "roles/datastore.user"
  member     = "serviceAccount:${google_service_account.hub_runtime.email}"
  depends_on = [time_sleep.wait_for_apis]
}

resource "google_project_iam_member" "hub_logging" {
  project = var.project_id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.hub_runtime.email}"
}

resource "google_project_iam_member" "hub_monitoring" {
  project = var.project_id
  role    = "roles/monitoring.metricWriter"
  member  = "serviceAccount:${google_service_account.hub_runtime.email}"
}

resource "google_service_account_iam_member" "hub_token_creator" {
  service_account_id = google_service_account.hub_runtime.name
  role               = "roles/iam.serviceAccountOpenIdTokenCreator"
  member             = "serviceAccount:${google_service_account.hub_runtime.email}"
}

resource "google_cloud_run_v2_service" "hub" {
  provider = google-beta
  project  = var.project_id
  name     = "hub"
  location = var.region

  launch_stage = "BETA"

  # Security Note: Default is INGRESS_TRAFFIC_ALL for initial development.
  # Phased rollout to INGRESS_TRAFFIC_INTERNAL_ONLY or INGRESS_TRAFFIC_INTERNAL_AND_GCLB.
  ingress = var.ingress_type

  # Enable Direct IAP (Preview feature)
  iap_enabled = var.iap_enabled

  custom_audiences = var.custom_audiences

  deletion_protection = false

  depends_on = [time_sleep.wait_for_apis]

  template {
    service_account = google_service_account.hub_runtime.email

    containers {
      image = "${var.region}-docker.pkg.dev/${var.image_project_id}/hub/hub:${var.hub_image_tag == "" ? "latest" : var.hub_image_tag}"

      env {
        name  = "ENVIRONMENT"
        value = var.environment
      }

      env {
        name  = "GCP_PROJECT_ID"
        value = var.firestore_project_id
      }

      env {
        name  = "GCP_LOCATION"
        value = var.region
      }

      env {
        name  = "PLATFORM_TRUST_TEAM"
        value = var.platform_trust_team
      }

      env {
        name  = "IAM_DOMAIN"
        value = var.iam_domain
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

      ports {
        container_port = 8080
        name           = "h2c"
      }
    }
  }
}

# Metadata Shovel Service Account
resource "google_service_account" "shovel_runtime" {
  project      = var.project_id
  account_id   = "metadata-shovel-sa"
  display_name = "Metadata Shovel Runtime Service Account"
}

# Grant Shovel access to Publish to Control Plane Topic
resource "google_pubsub_topic_iam_member" "shovel_control_plane_publisher" {
  project = var.project_id
  topic   = var.control_plane_topic
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.shovel_runtime.email}"
}

# Grant Shovel access to Delete Firestore Documents (cleanup)
resource "google_project_iam_member" "shovel_firestore_user" {
  project = var.firestore_project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.shovel_runtime.email}"
}

# Metadata Shovel Cloud Run Service
resource "google_cloud_run_v2_service" "shovel" {
  provider            = google-beta
  project             = var.project_id
  name                = "metadata-shovel"
  location            = var.region
  ingress             = "INGRESS_TRAFFIC_INTERNAL_ONLY" # Triggered by Eventarc only
  deletion_protection = false

  template {
    service_account = google_service_account.shovel_runtime.email
    containers {
      image = "${var.region}-docker.pkg.dev/${var.image_project_id}/metadata-shovel/metadata-shovel:${var.shovel_image_tag == "" ? "latest" : var.shovel_image_tag}"
      env {
        name  = "TARGET_TOPIC"
        value = var.control_plane_topic
      }
      env {
        name  = "EVENTS_TOPIC"
        value = google_pubsub_topic.events.name
      }
      env {
        name  = "GCP_PROJECT_ID"
        value = var.project_id
      }
      env {
        name  = "OIDC_AUDIENCE"
        value = google_cloud_run_v2_service.shovel.uri
      }
      env {
        name  = "AUTH_ENFORCE"
        value = var.auth_enforce ? "true" : "false"
      }
      env {
        name  = "TOPIC_ROUTING"
        value = jsonencode(var.topic_routing)
      }
      env {
        name  = "FORCE_DEPLOY"
        value = var.force_deploy_trigger
      }
    }
  }
}

# Grant Eventarc access to invoke Shovel
resource "google_cloud_run_v2_service_iam_member" "eventarc_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.shovel.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.eventarc_trigger_sa.email}"
}

# Service Account for Eventarc Trigger
resource "google_service_account" "eventarc_trigger_sa" {
  project      = var.project_id
  account_id   = "eventarc-trigger-sa"
  display_name = "Eventarc Trigger Service Account"
}

# Grant Eventarc SA access to receive events (Event Receiver)
resource "google_project_iam_member" "eventarc_receiver" {
  project = var.project_id
  role    = "roles/eventarc.eventReceiver"
  member  = "serviceAccount:${google_service_account.eventarc_trigger_sa.email}"
}

resource "google_project_service_identity" "eventarc_sa" {
  provider = google-beta
  project  = var.project_id
  service  = "eventarc.googleapis.com"

  depends_on = [google_project_service.hub_apis]
}

# Grant Eventarc Service Agent roles/pubsub.publisher
resource "google_project_iam_member" "eventarc_pubsub_publisher" {
  project = var.project_id
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_project_service_identity.eventarc_sa.email}"
}

# Eventarc Trigger for Firestore /outbox
resource "google_eventarc_trigger" "outbox_trigger" {
  depends_on              = [google_project_iam_member.eventarc_pubsub_publisher]
  project                 = var.project_id
  name                    = "metadata-shovel-trigger"
  location                = var.region
  event_data_content_type = "application/protobuf"

  matching_criteria {
    attribute = "type"
    value     = "google.cloud.firestore.document.v1.created"
  }

  matching_criteria {
    attribute = "database"
    value     = "(default)"
  }

  matching_criteria {
    attribute = "document"
    value     = "outbox/{document}"
    operator  = "match-path-pattern"
  }

  destination {
    cloud_run_service {
      service = google_cloud_run_v2_service.shovel.name
      region  = var.region
    }
  }

  service_account = google_service_account.eventarc_trigger_sa.email
}

resource "google_cloud_run_v2_service_iam_member" "additional_invokers" {
  for_each = toset(var.authorized_invokers)
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.hub.name
  role     = "roles/run.invoker"
  member   = each.value
}

output "service_url" {
  value = google_cloud_run_v2_service.hub.uri
}

output "service_name" {
  value = google_cloud_run_v2_service.hub.name
}

output "deployment_project_id" {
  value = var.project_id
}

output "iap_client_id" {
  value = var.iap_client_id
}

output "contract_changed_topic_id" {
  value = google_pubsub_topic.contract_changed.id
}

output "contract_deleted_topic_id" {
  value = google_pubsub_topic.contract_deleted.id
}

output "shovel_service_account_email" {
  description = "The service account email of the Metadata Shovel service"
  value       = google_service_account.shovel_runtime.email
}

# --- Event-Driven Alert Relay Infrastructure ---

resource "google_pubsub_topic" "events" {
  project = var.project_id
  name    = "supercargo-events"
}

resource "google_pubsub_topic" "contract_changed" {
  project = var.project_id
  name    = "catalog-contract-changed"
}

resource "google_pubsub_topic" "contract_deleted" {
  project = var.project_id
  name    = "catalog-contract-deleted"
}

resource "google_pubsub_topic_iam_member" "shovel_events_publisher" {
  project = var.project_id
  topic   = google_pubsub_topic.events.name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.shovel_runtime.email}"
}

resource "google_pubsub_topic_iam_member" "shovel_contract_changed_publisher" {
  project = var.project_id
  topic   = google_pubsub_topic.contract_changed.name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.shovel_runtime.email}"
}

resource "google_pubsub_topic_iam_member" "shovel_contract_deleted_publisher" {
  project = var.project_id
  topic   = google_pubsub_topic.contract_deleted.name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.shovel_runtime.email}"
}

resource "google_pubsub_topic_iam_member" "hub_contract_changed_publisher" {
  project = var.project_id
  topic   = google_pubsub_topic.contract_changed.name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.hub_runtime.email}"
}

resource "google_pubsub_topic_iam_member" "hub_contract_deleted_publisher" {
  project = var.project_id
  topic   = google_pubsub_topic.contract_deleted.name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.hub_runtime.email}"
}

resource "google_service_account" "events_invoker" {
  project      = var.project_id
  account_id   = "hub-events-invoker"
  display_name = "Hub Alert Relay Push Invoker"
}

resource "google_cloud_run_v2_service_iam_member" "events_invoker_run" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.hub.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.events_invoker.email}"
}

resource "google_pubsub_subscription" "events_push" {
  project = var.project_id
  name    = "hub-events-push"
  topic   = google_pubsub_topic.events.name

  ack_deadline_seconds = 60

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.hub.uri}/v1/internal/events"
    oidc_token {
      service_account_email = google_service_account.events_invoker.email
      audience              = google_cloud_run_v2_service.hub.uri
    }
  }

  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }
}
