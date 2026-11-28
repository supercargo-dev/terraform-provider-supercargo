locals {
  bq_location                     = var.bigquery_location != "" ? var.bigquery_location : var.region
  default_dataform_sink_filter    = "resource.type = \"dataform.googleapis.com/Repository\" AND (jsonPayload.terminalState:* OR protoPayload.@type = \"type.googleapis.com/google.cloud.dataform.v1alpha2.WorkflowInvocation\" OR jsonPayload.@type = \"type.googleapis.com/google.cloud.dataform.logging.v1.WorkflowInvocationCompletionLogEntry\" OR jsonPayload.@type = \"type.googleapis.com/google.cloud.dataform.v1alpha2.WorkflowInvocationCompletionLogEntry\")"
  default_job_sink_filter         = "resource.type = \"cloud_run_job\" AND (log_name =~ \"run.googleapis.com%2Fjob-execution-result\" OR textPayload =~ \"Completed with status:\")"
  console_iap_audience            = var.console_iap_audience != "" ? var.console_iap_audience : (var.console_iap_enabled ? "/projects/${var.project_number}/locations/${var.region}/services/console${var.name_suffix != "" ? "-${var.name_suffix}" : ""}" : "")
  console_telemetry_identity_salt = var.console_telemetry_identity_salt != "" ? var.console_telemetry_identity_salt : "supercargo-${var.project_id}-${var.environment}-telemetry-salt"
}

data "google_project" "project" {
  project_id = var.project_id
}

resource "google_project_service" "hub_apis" {
  for_each = toset([
    "bigquery.googleapis.com",
    "run.googleapis.com",
    "firestore.googleapis.com",
    "datastore.googleapis.com",
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
    "eventarc.googleapis.com",
    "datalineage.googleapis.com",
    "dataplex.googleapis.com", // Added Dataplex API
    "monitoring.googleapis.com",
    "logging.googleapis.com",
    "pubsub.googleapis.com",
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

// Grant Hub access to view lineage
resource "google_project_iam_member" "hub_datalineage_viewer" {
  project    = var.project_id
  role       = "roles/datalineage.viewer"
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

# Grant Gateway Service Account access to invoke Hub (IAM auth)
resource "google_cloud_run_v2_service_iam_member" "gateway_invoker" {
  count    = var.create_gateway_invoker ? 1 : 0
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

resource "google_firestore_index" "health_transitions_index" {
  project    = var.project_id
  collection = "health_transitions"

  fields {
    field_path = "urn"
    order      = "ASCENDING"
  }

  fields {
    field_path = "last_updated_at"
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

resource "google_firestore_index" "asset_incidents_by_asset_started_index" {
  project    = var.project_id
  collection = "asset_incidents"

  fields {
    field_path = "asset_urn"
    order      = "ASCENDING"
  }

  fields {
    field_path = "started_at"
    order      = "DESCENDING"
  }

  fields {
    field_path = "__name__"
    order      = "DESCENDING"
  }

  depends_on = [google_firestore_database.database]
}

resource "google_firestore_index" "asset_incidents_by_status_started_index" {
  project    = var.project_id
  collection = "asset_incidents"

  fields {
    field_path = "status"
    order      = "ASCENDING"
  }

  fields {
    field_path = "started_at"
    order      = "DESCENDING"
  }

  fields {
    field_path = "__name__"
    order      = "DESCENDING"
  }

  depends_on = [google_firestore_database.database]
}

resource "google_firestore_index" "asset_incidents_by_asset_status_started_index" {
  project    = var.project_id
  collection = "asset_incidents"

  fields {
    field_path = "asset_urn"
    order      = "ASCENDING"
  }

  fields {
    field_path = "status"
    order      = "ASCENDING"
  }

  fields {
    field_path = "started_at"
    order      = "DESCENDING"
  }

  fields {
    field_path = "__name__"
    order      = "DESCENDING"
  }

  depends_on = [google_firestore_database.database]
}

resource "google_firestore_index" "asset_incidents_by_category_started_index" {
  project    = var.project_id
  collection = "asset_incidents"

  fields {
    field_path = "category"
    order      = "ASCENDING"
  }

  fields {
    field_path = "started_at"
    order      = "DESCENDING"
  }

  fields {
    field_path = "__name__"
    order      = "DESCENDING"
  }

  depends_on = [google_firestore_database.database]
}

resource "google_firestore_index" "asset_incidents_by_backlink_started_index" {
  project    = var.project_id
  collection = "asset_incidents"

  fields {
    field_path = "backlink_status"
    order      = "ASCENDING"
  }

  fields {
    field_path = "started_at"
    order      = "DESCENDING"
  }

  fields {
    field_path = "__name__"
    order      = "DESCENDING"
  }

  depends_on = [google_firestore_database.database]
}

resource "google_firestore_index" "asset_incident_active_index" {
  project    = var.project_id
  collection = "asset_incident_active"

  fields {
    field_path = "asset_urn"
    order      = "ASCENDING"
  }

  fields {
    field_path = "status"
    order      = "ASCENDING"
  }

  fields {
    field_path = "started_at"
    order      = "DESCENDING"
  }

  fields {
    field_path = "__name__"
    order      = "DESCENDING"
  }

  depends_on = [google_firestore_database.database]
}

resource "google_firestore_index" "asset_incident_active_key_index" {
  project    = var.project_id
  collection = "asset_incident_active"

  fields {
    field_path = "key_hash"
    order      = "ASCENDING"
  }

  fields {
    field_path = "asset_urn"
    order      = "ASCENDING"
  }

  fields {
    field_path = "status"
    order      = "ASCENDING"
  }

  fields {
    field_path = "started_at"
    order      = "DESCENDING"
  }

  fields {
    field_path = "__name__"
    order      = "DESCENDING"
  }

  depends_on = [google_firestore_database.database]
}

resource "time_sleep" "wait_for_apis" {
  depends_on = [google_firestore_database.database]

  create_duration = "60s"
}

resource "google_service_account" "hub_runtime" {
  project      = var.project_id
  account_id   = "hub-runtime-sa${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
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
  name     = "hub${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  location = var.region

  launch_stage = "BETA"

  # Security Note: Default is INGRESS_TRAFFIC_ALL for initial development.
  # Phased rollout to INGRESS_TRAFFIC_INTERNAL_ONLY or INGRESS_TRAFFIC_INTERNAL_AND_GCLB.
  ingress = var.ingress_type

  custom_audiences = var.custom_audiences

  deletion_protection = false

  depends_on = [
    time_sleep.wait_for_apis,
    google_project_iam_member.hub_firestore_user,
  ]

  template {
    service_account = google_service_account.hub_runtime.email

    scaling {
      min_instance_count = 0
      max_instance_count = var.max_instances
    }

    containers {
      image = "${var.region}-docker.pkg.dev/${var.image_project_id}/hub/hub:${var.hub_image_tag == "" ? "latest" : var.hub_image_tag}"

      resources {
        cpu_idle = true
        limits = {
          cpu    = "1000m"
          memory = "512Mi"
        }
      }

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

      env {
        name  = "HEALTH_EVENTS_TOPIC"
        value = var.health_events_topic
      }

      env {
        name  = "HUB_PUSH_INVOKER_SA"
        value = google_service_account.events_invoker.email
      }

      env {
        name  = "LINEAGE_BACKPUSH_ENABLED"
        value = tostring(var.lineage_backpush_enabled)
      }

      env {
        name  = "LINEAGE_KEEPALIVE_TTL"
        value = var.lineage_keepalive_ttl
      }

      env {
        name  = "LINEAGE_KEEPALIVE_CHECK_INTERVAL"
        value = var.lineage_keepalive_check_interval
      }

      env {
        name  = "DATAPLEX_STREAMING_ENABLED"
        value = tostring(var.dataplex_streaming_enabled)
      }

      env {
        name  = "LINEAGE_HISTORY_DATASET"
        value = var.enable_lineage_history ? google_bigquery_dataset.supercargo_catalog[0].dataset_id : "supercargo_catalog"
      }

      env {
        name  = "LINEAGE_HISTORY_TABLE"
        value = var.lineage_history_table_id
      }

      ports {
        container_port = 8080
        name           = "h2c"
      }

      startup_probe {
        http_get {
          path = "/healthz"
          port = 8080
        }
        initial_delay_seconds = 0
        period_seconds        = 5
        timeout_seconds       = 2
        failure_threshold     = 12
      }

      liveness_probe {
        http_get {
          path = "/healthz"
          port = 8080
        }
        period_seconds    = 10
        timeout_seconds   = 2
        failure_threshold = 3
      }
    }
  }

  traffic {
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
    percent = 100
  }

  lifecycle {
    ignore_changes = [
      template[0].containers[0].image,
      client,
      client_version
    ]
  }
}

# Metadata Shovel Service Account
resource "google_service_account" "shovel_runtime" {
  project      = var.project_id
  account_id   = "metadata-shovel-sa${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
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
  name                = "metadata-shovel${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  location            = var.region
  ingress             = "INGRESS_TRAFFIC_INTERNAL_ONLY" # Triggered by Eventarc only
  deletion_protection = false

  template {
    service_account = google_service_account.shovel_runtime.email

    scaling {
      min_instance_count = 0
      max_instance_count = var.shovel_max_instances
    }

    containers {
      image = "${var.region}-docker.pkg.dev/${var.image_project_id}/metadata-shovel/metadata-shovel:${var.shovel_image_tag == "" ? "latest" : var.shovel_image_tag}"

      resources {
        cpu_idle = true
        limits = {
          cpu    = "1000m"
          memory = "512Mi"
        }
      }
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
        name  = "SHOVEL_OIDC_AUDIENCE"
        value = var.shovel_oidc_audience != "" ? var.shovel_oidc_audience : (var.oidc_audience != "" ? var.oidc_audience : "supercargo-metadata-shovel-${var.environment}")
      }
      env {
        name  = "AUTH_ENFORCE"
        value = var.auth_enforce ? "true" : "false"
      }
      env {
        name  = "INCIDENT_EVENTS_TOPIC"
        value = var.enable_health_mesh ? google_pubsub_topic.incident_events[0].name : ""
      }
      env {
        name = "TOPIC_ROUTING"
        value = jsonencode(merge(
          var.enable_lineage_history ? {
            "urn:supercargo:hub:contract:lineage-submission-event" = google_pubsub_topic.lineage_events[0].name
            "urn:supercargo:hub:contract:field-edge"               = google_pubsub_topic.field_edges[0].name
          } : {},
          var.enable_health_mesh ? { "urn:supercargo:hub:contract:asset-incident" = google_pubsub_topic.incident_events[0].name } : {},
          var.topic_routing
        ))
      }
      env {
        name  = "FORCE_DEPLOY"
        value = var.force_deploy_trigger
      }
    }
  }

  traffic {
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
    percent = 100
  }

  lifecycle {
    ignore_changes = [
      template[0].containers[0].image,
      template[0].containers[0].env,
      client,
      client_version
    ]
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
  account_id   = "eventarc-sa${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
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
  depends_on = [
    google_project_iam_member.eventarc_pubsub_publisher,
    google_project_iam_member.eventarc_receiver,
  ]
  project                 = var.project_id
  name                    = "shovel-trigger${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
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

resource "google_cloud_run_v2_service_iam_member" "openlineage_invokers" {
  for_each = toset([
    for sa in compact(var.openlineage_invoker_service_accounts) :
    startswith(trimspace(sa), "serviceAccount:") ? trimspace(sa) : "serviceAccount:${trimspace(sa)}"
  ])
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.hub.name
  role     = "roles/run.invoker"
  member   = each.value
}

# --- BigQuery Audit Sink Infrastructure ---

resource "google_bigquery_dataset" "supercargo_audit" {
  count                      = var.enable_audit_sink ? 1 : 0
  project                    = var.project_id
  dataset_id                 = var.audit_dataset_id
  location                   = local.bq_location
  delete_contents_on_destroy = !var.bigquery_deletion_protection

  depends_on = [google_project_service.hub_apis]
}

resource "google_bigquery_table" "outbox_events" {
  count               = var.enable_audit_sink ? 1 : 0
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.supercargo_audit[0].dataset_id
  table_id            = var.audit_table_id
  deletion_protection = var.bigquery_deletion_protection

  time_partitioning {
    type  = "DAY"
    field = "publish_time"
  }

  schema = jsonencode([
    {
      name        = "subscription_name"
      type        = "STRING"
      mode        = "REQUIRED"
      description = "Name of the Pub/Sub subscription delivering the event"
    },
    {
      name        = "message_id"
      type        = "STRING"
      mode        = "REQUIRED"
      description = "Unique Pub/Sub message ID"
    },
    {
      name        = "publish_time"
      type        = "TIMESTAMP"
      mode        = "REQUIRED"
      description = "Timestamp when message was published to Pub/Sub"
    },
    {
      name        = "attributes"
      type        = "JSON"
      mode        = "NULLABLE"
      description = "Pub/Sub message attributes map as JSON"
    },
    {
      name        = "data"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Raw message payload string"
    }
  ])
}

resource "google_bigquery_table" "outbox_events_view" {
  count               = var.enable_audit_sink ? 1 : 0
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.supercargo_audit[0].dataset_id
  table_id            = var.audit_view_id
  deletion_protection = false

  view {
    query          = <<EOF
SELECT
  COALESCE(JSON_VALUE(attributes, '$.doc_id'), message_id) AS event_id,
  JSON_VALUE(attributes, '$.urn') AS urn,
  JSON_VALUE(attributes, '$.action') AS action,
  JSON_VALUE(attributes, '$.doc_id') AS doc_id,
  SAFE.PARSE_JSON(data) AS payload,
  publish_time AS timestamp
FROM `${var.project_id}.${google_bigquery_dataset.supercargo_audit[0].dataset_id}.${google_bigquery_table.outbox_events[0].table_id}`
EOF
    use_legacy_sql = false
  }
}

resource "google_bigquery_dataset_iam_member" "pubsub_audit_bq_writer" {
  count      = var.enable_audit_sink ? 1 : 0
  project    = var.project_id
  dataset_id = google_bigquery_dataset.supercargo_audit[0].dataset_id
  role       = "roles/bigquery.dataEditor"
  member     = "serviceAccount:service-${var.project_number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_bigquery_dataset_iam_member" "pubsub_audit_bq_metadata" {
  count      = var.enable_audit_sink ? 1 : 0
  project    = var.project_id
  dataset_id = google_bigquery_dataset.supercargo_audit[0].dataset_id
  role       = "roles/bigquery.metadataViewer"
  member     = "serviceAccount:service-${var.project_number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_pubsub_subscription" "outbox_audit_bq" {
  count   = var.enable_audit_sink ? 1 : 0
  project = var.project_id
  name    = "supercargo-audit-bq${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  topic   = var.control_plane_topic

  expiration_policy {
    ttl = ""
  }

  bigquery_config {
    table               = "${var.project_id}.${google_bigquery_dataset.supercargo_audit[0].dataset_id}.${google_bigquery_table.outbox_events[0].table_id}"
    use_topic_schema    = false
    use_table_schema    = false
    write_metadata      = true
    drop_unknown_fields = true
  }

  depends_on = [
    google_bigquery_dataset_iam_member.pubsub_audit_bq_writer,
    google_bigquery_dataset_iam_member.pubsub_audit_bq_metadata
  ]
}

# --- Lineage-Enriched Health Event Mesh & BigQuery Subscription Infrastructure ---

resource "google_pubsub_topic" "health_events" {
  count   = var.enable_health_mesh ? 1 : 0
  project = var.project_id
  name    = var.health_events_topic
}

resource "google_pubsub_topic_iam_member" "hub_health_events_publisher" {
  count   = var.enable_health_mesh ? 1 : 0
  project = var.project_id
  topic   = google_pubsub_topic.health_events[0].name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.hub_runtime.email}"
}

resource "google_bigquery_dataset" "supercargo_catalog" {
  count                      = (var.enable_health_mesh || var.enable_lineage_history) ? 1 : 0
  project                    = var.project_id
  dataset_id                 = var.catalog_dataset_id
  location                   = local.bq_location
  delete_contents_on_destroy = !var.bigquery_deletion_protection

  depends_on = [google_project_service.hub_apis]
}

resource "google_bigquery_dataset_iam_member" "pubsub_health_bq_writer" {
  count      = (var.enable_health_mesh || var.enable_lineage_history) ? 1 : 0
  project    = var.project_id
  dataset_id = google_bigquery_dataset.supercargo_catalog[0].dataset_id
  role       = "roles/bigquery.dataEditor"
  member     = "serviceAccount:service-${var.project_number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_bigquery_dataset_iam_member" "pubsub_health_bq_metadata" {
  count      = (var.enable_health_mesh || var.enable_lineage_history) ? 1 : 0
  project    = var.project_id
  dataset_id = google_bigquery_dataset.supercargo_catalog[0].dataset_id
  role       = "roles/bigquery.metadataViewer"
  member     = "serviceAccount:service-${var.project_number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_bigquery_table" "asset_health_history" {
  count               = var.enable_health_mesh ? 1 : 0
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.supercargo_catalog[0].dataset_id
  table_id            = var.health_history_table_id
  deletion_protection = var.bigquery_deletion_protection

  time_partitioning {
    type  = "DAY"
    field = "timestamp"
  }

  clustering = ["asset_urn", "current_state"]

  schema = jsonencode([
    {
      name        = "event_id"
      type        = "STRING"
      mode        = "REQUIRED"
      description = "Unique transition event identifier"
    },
    {
      name        = "asset_urn"
      type        = "STRING"
      mode        = "REQUIRED"
      description = "URN of the asset experiencing the health transition"
    },
    {
      name        = "previous_state"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Previous health state before this transition"
    },
    {
      name        = "current_state"
      type        = "STRING"
      mode        = "REQUIRED"
      description = "Current health state after transition"
    },
    {
      name        = "reason"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Operational reason or error message for transition"
    },
    {
      name        = "incident_type"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Classification of the incident"
    },
    {
      name        = "run_id"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Pipeline execution or orchestration run ID"
    },
    {
      name        = "reporter"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Entity or identity reporting the anomaly"
    },
    {
      name        = "status_source_urn"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Root cause asset URN triggering the status change"
    },
    {
      name        = "timestamp"
      type        = "TIMESTAMP"
      mode        = "REQUIRED"
      description = "Timestamp when the health transition occurred"
    },
    {
      name        = "downstream_urns"
      type        = "STRING"
      mode        = "REPEATED"
      description = "List of direct and transitive downstream assets impacted"
    },
    {
      name        = "impacted_teams"
      type        = "STRING"
      mode        = "REPEATED"
      description = "List of owner teams impacted across the downstream blast radius"
    },
    {
      name        = "subscription_name"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Name of the Pub/Sub subscription delivering the event"
    },
    {
      name        = "message_id"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Pub/Sub message ID"
    },
    {
      name        = "publish_time"
      type        = "TIMESTAMP"
      mode        = "NULLABLE"
      description = "Pub/Sub message publish timestamp"
    },
    {
      name        = "attributes"
      type        = "JSON"
      mode        = "NULLABLE"
      description = "Pub/Sub message attributes map"
    }
  ])
}

resource "google_pubsub_subscription" "health_events_bq" {
  count   = var.enable_health_mesh ? 1 : 0
  project = var.project_id
  name    = "supercargo-health-events-bq${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  topic   = google_pubsub_topic.health_events[0].name

  expiration_policy {
    ttl = ""
  }

  bigquery_config {
    table               = "${var.project_id}.${google_bigquery_dataset.supercargo_catalog[0].dataset_id}.${google_bigquery_table.asset_health_history[0].table_id}"
    use_table_schema    = true
    write_metadata      = true
    drop_unknown_fields = true
  }

  depends_on = [
    google_bigquery_dataset_iam_member.pubsub_health_bq_writer,
    google_bigquery_dataset_iam_member.pubsub_health_bq_metadata
  ]
}

resource "google_pubsub_topic" "incident_events" {
  count   = var.enable_health_mesh ? 1 : 0
  project = var.project_id
  name    = "${var.incident_events_topic}${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
}

resource "google_pubsub_topic" "incident_events_dlq" {
  count   = var.enable_health_mesh ? 1 : 0
  project = var.project_id
  name    = "${var.incident_events_topic}-dlq${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
}

resource "google_pubsub_topic_iam_member" "shovel_incident_events_publisher" {
  count   = var.enable_health_mesh ? 1 : 0
  project = var.project_id
  topic   = google_pubsub_topic.incident_events[0].name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.shovel_runtime.email}"
}

resource "google_pubsub_topic_iam_member" "hub_incident_events_publisher" {
  count   = var.enable_health_mesh ? 1 : 0
  project = var.project_id
  topic   = google_pubsub_topic.incident_events[0].name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.hub_runtime.email}"
}

resource "google_pubsub_topic_iam_member" "incident_dlq_publisher" {
  count   = var.enable_health_mesh ? 1 : 0
  project = var.project_id
  topic   = google_pubsub_topic.incident_events_dlq[0].name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:service-${var.project_number != "" ? var.project_number : data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_bigquery_table" "asset_incident_events" {
  count               = var.enable_health_mesh ? 1 : 0
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.supercargo_catalog[0].dataset_id
  table_id            = var.incident_events_table_id
  deletion_protection = var.bigquery_deletion_protection

  time_partitioning {
    type  = "DAY"
    field = "started_at"
  }

  clustering = ["asset_urn", "category", "status"]

  schema = jsonencode([
    {
      name        = "event_id"
      type        = "STRING"
      mode        = "REQUIRED"
      description = "Unique incident CDC outbox event identifier"
    },
    {
      name        = "incident_id"
      type        = "STRING"
      mode        = "REQUIRED"
      description = "Immutable bounded incident interval identifier"
    },
    {
      name        = "asset_urn"
      type        = "STRING"
      mode        = "REQUIRED"
      description = "Canonical URN of the affected data asset"
    },
    {
      name        = "category"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Canonical incident root category (SCHEMA, ASSERTION, ANOMALY, SLA, SEMANTIC, INFRA, MANUAL)"
    },
    {
      name        = "subtype"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Operational incident subtype (DBT, DATAFORM, BENTO_DLQ, OPENLINEAGE, TIMESFM, etc.)"
    },
    {
      name        = "incident_type"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Original or normalized CATEGORY:SUBTYPE classification"
    },
    {
      name        = "reporter"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Reporting subsystem or authenticated human operator identity"
    },
    {
      name        = "status"
      type        = "STRING"
      mode        = "REQUIRED"
      description = "Incident lifecycle state (INCIDENT_STATUS_OPEN, INCIDENT_STATUS_ACKNOWLEDGED, INCIDENT_STATUS_CLOSED)"
    },
    {
      name        = "health_state"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Health state projected by this incident event"
    },
    {
      name        = "reason"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Scrubbed diagnostic reason for the incident"
    },
    {
      name        = "run_id"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Latest orchestrator or pipeline run ID associated with the incident"
    },
    {
      name        = "coalesced_count"
      type        = "INT64"
      mode        = "NULLABLE"
      description = "Number of repeated failures coalesced into this bounded interval"
    },
    {
      name        = "started_at"
      type        = "TIMESTAMP"
      mode        = "REQUIRED"
      description = "Timestamp when the bounded incident interval opened"
    },
    {
      name        = "last_seen_at"
      type        = "TIMESTAMP"
      mode        = "NULLABLE"
      description = "Timestamp of the most recent failure coalesced into the interval"
    },
    {
      name        = "ended_at"
      type        = "TIMESTAMP"
      mode        = "NULLABLE"
      description = "Timestamp when the bounded incident interval closed"
    },
    {
      name        = "duration_seconds"
      type        = "INT64"
      mode        = "NULLABLE"
      description = "Total elapsed outage duration in seconds once closed"
    },
    {
      name        = "resolution_type"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "How the incident was closed (AUTO_RECOVERED, MANUAL_OVERRIDE, FALSE_ALARM)"
    },
    {
      name        = "override_reason"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Operator justification when closed manually"
    },
    {
      name        = "root_cause_category"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Post-triage root cause classification"
    },
    {
      name        = "audit_actor"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Server-stamped identity of the actor opening or closing the incident"
    },
    {
      name        = "acknowledged_by"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Server-stamped identity of the operator who acknowledged the incident"
    },
    {
      name        = "acknowledged_at"
      type        = "TIMESTAMP"
      mode        = "NULLABLE"
      description = "Timestamp when the incident was acknowledged"
    },
    {
      name        = "backlink_status"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "TimesFM false-negative attribution status"
    },
    {
      name        = "linked_snapshot_id"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Linked TimesFM feature window snapshot identifier"
    },
    {
      name        = "linked_window_start"
      type        = "TIMESTAMP"
      mode        = "NULLABLE"
      description = "Start of the back-linked TimesFM feature window"
    },
    {
      name        = "linked_window_end"
      type        = "TIMESTAMP"
      mode        = "NULLABLE"
      description = "End of the back-linked TimesFM feature window"
    },
    {
      name        = "metadata_json"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "JSON-encoded incident metadata map (including dbt failing_tests)"
    },
    {
      name        = "emitted_at"
      type        = "TIMESTAMP"
      mode        = "REQUIRED"
      description = "Timestamp when the CDC outbox event was emitted"
    },
    {
      name        = "subscription_name"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Name of the Pub/Sub subscription delivering the event"
    },
    {
      name        = "message_id"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Pub/Sub message ID"
    },
    {
      name        = "publish_time"
      type        = "TIMESTAMP"
      mode        = "NULLABLE"
      description = "Pub/Sub message publish timestamp"
    },
    {
      name        = "attributes"
      type        = "JSON"
      mode        = "NULLABLE"
      description = "Pub/Sub message attributes map"
    }
  ])
}

resource "google_pubsub_subscription" "incident_events_bq" {
  count   = var.enable_health_mesh ? 1 : 0
  project = var.project_id
  name    = "supercargo-incident-events-bq${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  topic   = google_pubsub_topic.incident_events[0].name

  expiration_policy {
    ttl = ""
  }

  bigquery_config {
    table               = "${var.project_id}.${google_bigquery_dataset.supercargo_catalog[0].dataset_id}.${google_bigquery_table.asset_incident_events[0].table_id}"
    use_table_schema    = true
    write_metadata      = true
    drop_unknown_fields = true
  }

  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.incident_events_dlq[0].id
    max_delivery_attempts = 5
  }

  depends_on = [
    google_bigquery_dataset_iam_member.pubsub_health_bq_writer,
    google_bigquery_dataset_iam_member.pubsub_health_bq_metadata,
    google_pubsub_topic_iam_member.incident_dlq_publisher,
  ]
}

resource "google_pubsub_subscription_iam_member" "incident_bq_dlq_subscriber" {
  count        = var.enable_health_mesh ? 1 : 0
  project      = var.project_id
  subscription = google_pubsub_subscription.incident_events_bq[0].name
  role         = "roles/pubsub.subscriber"
  member       = "serviceAccount:service-${var.project_number != "" ? var.project_number : data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_pubsub_subscription" "incident_events_dlq_pull" {
  count   = var.enable_health_mesh ? 1 : 0
  project = var.project_id
  name    = "${var.incident_events_topic}-dlq-sub${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  topic   = google_pubsub_topic.incident_events_dlq[0].name

  message_retention_duration = "604800s"
  retain_acked_messages      = false

  expiration_policy {
    ttl = ""
  }
}

resource "google_bigquery_table" "asset_incidents" {
  count               = var.enable_health_mesh ? 1 : 0
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.supercargo_catalog[0].dataset_id
  table_id            = var.incidents_view_id
  deletion_protection = false

  view {
    query          = <<EOF
SELECT * EXCEPT(row_num)
FROM (
  SELECT
    *,
    ROW_NUMBER() OVER(PARTITION BY incident_id ORDER BY emitted_at DESC, publish_time DESC NULLS LAST) AS row_num
  FROM `${var.project_id}.${google_bigquery_dataset.supercargo_catalog[0].dataset_id}.${google_bigquery_table.asset_incident_events[0].table_id}`
)
WHERE row_num = 1
EOF
    use_legacy_sql = false
  }
}

resource "google_bigquery_table" "asset_current_health" {
  count               = var.enable_health_mesh ? 1 : 0
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.supercargo_catalog[0].dataset_id
  table_id            = var.current_health_view_id
  deletion_protection = false

  view {
    query          = <<EOF
WITH latest_health AS (
  SELECT * EXCEPT(row_num)
  FROM (
    SELECT *, ROW_NUMBER() OVER(PARTITION BY asset_urn ORDER BY timestamp DESC, publish_time DESC NULLS LAST) as row_num
    FROM `${var.project_id}.${google_bigquery_dataset.supercargo_catalog[0].dataset_id}.${google_bigquery_table.asset_health_history[0].table_id}`
  )
  WHERE row_num = 1
),
open_incidents_agg AS (
  SELECT
    asset_urn,
    COUNT(1) AS open_incident_count,
    ARRAY_AGG(DISTINCT reporter IGNORE NULLS) AS active_reporters,
    MIN(started_at) AS oldest_started_at,
    ARRAY_AGG(reason IGNORE NULLS ORDER BY started_at ASC LIMIT 1)[SAFE_OFFSET(0)] AS primary_incident_reason,
    ARRAY_AGG(incident_type IGNORE NULLS ORDER BY started_at ASC LIMIT 1)[SAFE_OFFSET(0)] AS primary_incident_type,
    ARRAY_AGG(reporter IGNORE NULLS ORDER BY started_at ASC LIMIT 1)[SAFE_OFFSET(0)] AS primary_reporter
  FROM `${var.project_id}.${google_bigquery_dataset.supercargo_catalog[0].dataset_id}.${google_bigquery_table.asset_incidents[0].table_id}`
  WHERE status IN ('INCIDENT_STATUS_OPEN', 'INCIDENT_STATUS_ACKNOWLEDGED', 'OPEN', 'ACKNOWLEDGED')
  GROUP BY asset_urn
),
all_urns AS (
  SELECT asset_urn FROM latest_health
  UNION DISTINCT
  SELECT asset_urn FROM open_incidents_agg
)
SELECT
  COALESCE(h.event_id, CONCAT('inc-proj-', u.asset_urn)) AS event_id,
  u.asset_urn AS asset_urn,
  h.previous_state AS previous_state,
  CASE
    WHEN COALESCE(o.open_incident_count, 0) > 0 THEN COALESCE(NULLIF(h.current_state, ''), 'HEALTH_STATE_DEGRADED')
    ELSE h.current_state
  END AS current_state,
  COALESCE(NULLIF(h.reason, ''), o.primary_incident_reason) AS reason,
  COALESCE(NULLIF(h.incident_type, ''), o.primary_incident_type) AS incident_type,
  h.run_id AS run_id,
  COALESCE(NULLIF(h.reporter, ''), o.primary_reporter) AS reporter,
  COALESCE(NULLIF(h.status_source_urn, ''), IF(COALESCE(o.open_incident_count, 0) > 0, u.asset_urn, NULL)) AS status_source_urn,
  COALESCE(h.timestamp, o.oldest_started_at) AS timestamp,
  IFNULL(h.downstream_urns, ARRAY<STRING>[]) AS downstream_urns,
  IFNULL(h.impacted_teams, ARRAY<STRING>[]) AS impacted_teams,
  h.subscription_name AS subscription_name,
  h.message_id AS message_id,
  h.publish_time AS publish_time,
  h.attributes AS attributes,
  CASE
    WHEN COALESCE(o.open_incident_count, 0) > 0 OR h.current_state IN ('DEGRADED', 'HEALTH_STATE_DEGRADED', 'UNHEALTHY', 'HEALTH_STATE_UNHEALTHY') THEN 'RED'
    WHEN h.current_state IN ('AT_RISK', 'HEALTH_STATE_AT_RISK') THEN 'YELLOW'
    ELSE 'GREEN'
  END AS traffic_light,
  COALESCE(NULLIF(h.status_source_urn, ''), IF(COALESCE(o.open_incident_count, 0) > 0, u.asset_urn, NULL)) AS root_cause_urn,
  COALESCE(o.open_incident_count, 0) AS open_incident_count,
  IFNULL(o.active_reporters, ARRAY<STRING>[]) AS active_reporters,
  o.oldest_started_at AS oldest_started_at
FROM all_urns u
LEFT JOIN latest_health h ON u.asset_urn = h.asset_urn
LEFT JOIN open_incidents_agg o ON u.asset_urn = o.asset_urn
EOF
    use_legacy_sql = false
  }
}

resource "google_bigquery_table" "v_timesfm_incident_feedback" {
  count               = var.enable_health_mesh ? 1 : 0
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.supercargo_catalog[0].dataset_id
  table_id            = var.timesfm_incident_feedback_view_id
  deletion_protection = false

  view {
    query          = <<EOF
SELECT
  incident_id,
  asset_urn,
  category,
  subtype,
  incident_type,
  reporter,
  status,
  health_state,
  reason,
  coalesced_count,
  started_at,
  last_seen_at,
  ended_at,
  duration_seconds,
  resolution_type,
  override_reason,
  root_cause_category,
  audit_actor,
  acknowledged_by,
  acknowledged_at,
  backlink_status,
  linked_snapshot_id,
  linked_window_start,
  linked_window_end,
  (backlink_status IN ('BACKLINK_STATUS_MISSED_BY_TIMESFM', 'MISSED_BY_TIMESFM')) AS is_missed_by_timesfm,
  (resolution_type IN ('RESOLUTION_TYPE_FALSE_ALARM', 'FALSE_ALARM')) AS is_false_alarm,
  emitted_at
FROM `${var.project_id}.${google_bigquery_dataset.supercargo_catalog[0].dataset_id}.${google_bigquery_table.asset_incidents[0].table_id}`
WHERE
  backlink_status IN (
    'BACKLINK_STATUS_MISSED_BY_TIMESFM',
    'MISSED_BY_TIMESFM',
    'BACKLINK_STATUS_MATCHED_EXISTING_ALERT',
    'MATCHED_EXISTING_ALERT'
  )
  OR resolution_type IN (
    'RESOLUTION_TYPE_FALSE_ALARM',
    'FALSE_ALARM',
    'RESOLUTION_TYPE_MANUAL_OVERRIDE',
    'MANUAL_OVERRIDE'
  )
  OR category = 'MANUAL'
EOF
    use_legacy_sql = false
  }
}

# --- Lineage Events History & BigQuery Subscription Infrastructure ---

resource "google_pubsub_topic" "lineage_events" {
  count   = var.enable_lineage_history ? 1 : 0
  project = var.project_id
  name    = "${var.lineage_events_topic}${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
}

resource "google_pubsub_topic" "lineage_events_dlq" {
  count   = var.enable_lineage_history ? 1 : 0
  project = var.project_id
  name    = "${var.lineage_events_topic}-dlq${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
}

resource "google_pubsub_topic_iam_member" "shovel_lineage_events_publisher" {
  count   = var.enable_lineage_history ? 1 : 0
  project = var.project_id
  topic   = google_pubsub_topic.lineage_events[0].name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.shovel_runtime.email}"
}

resource "google_pubsub_topic_iam_member" "hub_lineage_events_publisher" {
  count   = var.enable_lineage_history ? 1 : 0
  project = var.project_id
  topic   = google_pubsub_topic.lineage_events[0].name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.hub_runtime.email}"
}

resource "google_pubsub_topic_iam_member" "lineage_dlq_publisher" {
  count   = var.enable_lineage_history ? 1 : 0
  project = var.project_id
  topic   = google_pubsub_topic.lineage_events_dlq[0].name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:service-${var.project_number != "" ? var.project_number : data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_bigquery_table" "lineage_events_history" {
  count               = var.enable_lineage_history ? 1 : 0
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.supercargo_catalog[0].dataset_id
  table_id            = var.lineage_history_table_id
  deletion_protection = var.bigquery_deletion_protection

  time_partitioning {
    type  = "DAY"
    field = "event_time"
  }

  clustering = ["source_system", "granularity", "submission_id"]

  schema = jsonencode([
    {
      name        = "submission_id"
      type        = "STRING"
      mode        = "REQUIRED"
      description = "Unique identifier for the lineage submission"
    },
    {
      name        = "chunk_index"
      type        = "INT64"
      mode        = "REQUIRED"
      description = "Zero-based index of this submission chunk"
    },
    {
      name        = "chunk_total"
      type        = "INT64"
      mode        = "REQUIRED"
      description = "Total number of chunks in the submission"
    },
    {
      name        = "source_system"
      type        = "STRING"
      mode        = "REQUIRED"
      description = "Originating lineage plane (dbt, dataform, dataplex, openlineage, manifest)"
    },
    {
      name        = "granularity"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Execution granularity (MODEL, TASK, QUERY, PIPELINE)"
    },
    {
      name        = "event_time"
      type        = "TIMESTAMP"
      mode        = "REQUIRED"
      description = "Timestamp when the lineage submission occurred"
    },
    {
      name        = "actor"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Authenticated principal or service submitting the lineage event"
    },
    {
      name        = "product_urn"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Optional owning Data Product URN"
    },
    {
      name        = "run_id"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Orchestrator or pipeline run identifier"
    },
    {
      name        = "run_state"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Terminal run state (COMPLETE, FAIL, ABORT)"
    },
    {
      name        = "scope_id"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Scoped manifest or project identifier for stale-edge replacement"
    },
    {
      name        = "edges_json"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "JSON-encoded array of canonical table/asset edges in this chunk"
    },
    {
      name        = "field_edges_json"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "JSON-encoded array of canonical column-level field edges in this chunk"
    },
    {
      name        = "subscription_name"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Name of the Pub/Sub subscription delivering the event"
    },
    {
      name        = "message_id"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Pub/Sub message ID"
    },
    {
      name        = "publish_time"
      type        = "TIMESTAMP"
      mode        = "NULLABLE"
      description = "Pub/Sub message publish timestamp"
    },
    {
      name        = "attributes"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Pub/Sub message attributes map"
    }
  ])
}

resource "google_pubsub_subscription" "lineage_events_bq" {
  count   = var.enable_lineage_history ? 1 : 0
  project = var.project_id
  name    = "supercargo-lineage-events-bq${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  topic   = google_pubsub_topic.lineage_events[0].name

  expiration_policy {
    ttl = ""
  }

  bigquery_config {
    table               = "${var.project_id}.${google_bigquery_dataset.supercargo_catalog[0].dataset_id}.${google_bigquery_table.lineage_events_history[0].table_id}"
    use_table_schema    = true
    write_metadata      = true
    drop_unknown_fields = true
  }

  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.lineage_events_dlq[0].id
    max_delivery_attempts = 5
  }

  depends_on = [
    google_bigquery_dataset_iam_member.pubsub_health_bq_writer,
    google_bigquery_dataset_iam_member.pubsub_health_bq_metadata,
    google_pubsub_topic_iam_member.lineage_dlq_publisher,
  ]
}

resource "google_pubsub_subscription_iam_member" "lineage_bq_dlq_subscriber" {
  count        = var.enable_lineage_history ? 1 : 0
  project      = var.project_id
  subscription = google_pubsub_subscription.lineage_events_bq[0].name
  role         = "roles/pubsub.subscriber"
  member       = "serviceAccount:service-${var.project_number != "" ? var.project_number : data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_pubsub_subscription" "lineage_events_dlq_pull" {
  count   = var.enable_lineage_history ? 1 : 0
  project = var.project_id
  name    = "${var.lineage_events_topic}-dlq-sub${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  topic   = google_pubsub_topic.lineage_events_dlq[0].name

  message_retention_duration = "604800s"
  retain_acked_messages      = false

  expiration_policy {
    ttl = ""
  }
}

resource "google_bigquery_table" "lineage_current_edges" {
  count               = var.enable_lineage_history ? 1 : 0
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.supercargo_catalog[0].dataset_id
  table_id            = var.lineage_current_edges_view_id
  deletion_protection = false

  view {
    query          = <<EOF
SELECT
  submission_id,
  chunk_index,
  chunk_total,
  source_system,
  granularity,
  event_time,
  actor,
  product_urn,
  run_id,
  run_state,
  scope_id,
  SAFE.PARSE_JSON(edges_json) AS edges_json,
  SAFE.PARSE_JSON(field_edges_json) AS field_edges_json,
  publish_time
FROM (
  SELECT
    *,
    ROW_NUMBER() OVER(
      PARTITION BY submission_id, chunk_index
      ORDER BY event_time DESC, publish_time DESC NULLS LAST
    ) AS row_num
  FROM `${var.project_id}.${google_bigquery_dataset.supercargo_catalog[0].dataset_id}.${google_bigquery_table.lineage_events_history[0].table_id}`
)
WHERE row_num = 1
EOF
    use_legacy_sql = false
  }
}

# --- Column-Level Field Edges CDC & Recursive Field Closures View (#1381) ---

resource "google_pubsub_topic" "field_edges" {
  count   = var.enable_lineage_history ? 1 : 0
  project = var.project_id
  name    = "${var.field_edges_topic}${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
}

resource "google_pubsub_topic" "field_edges_dlq" {
  count   = var.enable_lineage_history ? 1 : 0
  project = var.project_id
  name    = "${var.field_edges_topic}-dlq${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
}

resource "google_pubsub_topic_iam_member" "shovel_field_edges_publisher" {
  count   = var.enable_lineage_history ? 1 : 0
  project = var.project_id
  topic   = google_pubsub_topic.field_edges[0].name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.shovel_runtime.email}"
}

resource "google_pubsub_topic_iam_member" "hub_field_edges_publisher" {
  count   = var.enable_lineage_history ? 1 : 0
  project = var.project_id
  topic   = google_pubsub_topic.field_edges[0].name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.hub_runtime.email}"
}

resource "google_pubsub_topic_iam_member" "field_edges_dlq_publisher" {
  count   = var.enable_lineage_history ? 1 : 0
  project = var.project_id
  topic   = google_pubsub_topic.field_edges_dlq[0].name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:service-${var.project_number != "" ? var.project_number : data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_bigquery_table" "field_edges" {
  count               = var.enable_lineage_history ? 1 : 0
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.supercargo_catalog[0].dataset_id
  table_id            = var.field_edges_table_id
  deletion_protection = var.bigquery_deletion_protection

  time_partitioning {
    type  = "DAY"
    field = "updated_at"
  }

  clustering = ["from_field_urn", "to_field_urn", "source_system"]

  schema = jsonencode([
    {
      name        = "from_field_urn"
      type        = "STRING"
      mode        = "REQUIRED"
      description = "The source field URN (upstream)"
    },
    {
      name        = "to_field_urn"
      type        = "STRING"
      mode        = "REQUIRED"
      description = "The destination field URN (downstream)"
    },
    {
      name        = "type"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Origin of the edge (MANAGED, NATIVE)"
    },
    {
      name        = "participants"
      type        = "STRING"
      mode        = "REPEATED"
      description = "Array containing both from_field_urn and to_field_urn"
    },
    {
      name        = "transformation_kind"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Type of transformation (e.g., IDENTITY, HASH, VIEW)"
    },
    {
      name        = "approved_mask"
      type        = "BOOL"
      mode        = "NULLABLE"
      description = "True if the transformation is approved for PII declassification"
    },
    {
      name        = "metadata"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "JSON-encoded provider-specific or audit metadata string"
    },
    {
      name        = "updated_at"
      type        = "TIMESTAMP"
      mode        = "REQUIRED"
      description = "Timestamp when the edge was last updated or reconciled"
    },
    {
      name        = "kind"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Semantic kind of the lineage edge (DATAFLOW, VIEW, REFERENCE, TRANSPORT)"
    },
    {
      name        = "confidence"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Certainty level of the lineage edge (OBSERVED, DECLARED, INFERRED)"
    },
    {
      name        = "granularity"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Granularity at which the edge was captured (COLUMN, QUERY, TASK, WORKFLOW)"
    },
    {
      name        = "source_system"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Originating platform or tool (manifest, dbt, dataform, dataplex, openlineage)"
    },
    {
      name        = "job_fqn"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Job or query FQN that produced this edge"
    },
    {
      name        = "first_seen_at"
      type        = "TIMESTAMP"
      mode        = "NULLABLE"
      description = "Timestamp when the edge was first observed or declared"
    },
    {
      name        = "last_observed_at"
      type        = "TIMESTAMP"
      mode        = "NULLABLE"
      description = "Timestamp when the edge was most recently observed"
    },
    {
      name        = "deleted_at"
      type        = "TIMESTAMP"
      mode        = "NULLABLE"
      description = "Soft-delete tombstone timestamp for incremental CDC"
    },
    {
      name        = "subscription_name"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Name of the Pub/Sub subscription delivering the event"
    },
    {
      name        = "message_id"
      type        = "STRING"
      mode        = "NULLABLE"
      description = "Pub/Sub message ID"
    },
    {
      name        = "publish_time"
      type        = "TIMESTAMP"
      mode        = "NULLABLE"
      description = "Pub/Sub message publish timestamp"
    },
    {
      name        = "attributes"
      type        = "JSON"
      mode        = "NULLABLE"
      description = "Pub/Sub message attributes map"
    }
  ])
}

resource "google_pubsub_subscription" "field_edges_bq" {
  count   = var.enable_lineage_history ? 1 : 0
  project = var.project_id
  name    = "supercargo-field-edges-bq${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  topic   = google_pubsub_topic.field_edges[0].name

  expiration_policy {
    ttl = ""
  }

  bigquery_config {
    table               = "${var.project_id}.${google_bigquery_dataset.supercargo_catalog[0].dataset_id}.${google_bigquery_table.field_edges[0].table_id}"
    use_table_schema    = true
    write_metadata      = true
    drop_unknown_fields = true
  }

  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.field_edges_dlq[0].id
    max_delivery_attempts = 5
  }

  depends_on = [
    google_bigquery_dataset_iam_member.pubsub_health_bq_writer,
    google_bigquery_dataset_iam_member.pubsub_health_bq_metadata,
    google_pubsub_topic_iam_member.field_edges_dlq_publisher,
  ]
}

resource "google_pubsub_subscription_iam_member" "field_edges_bq_dlq_subscriber" {
  count        = var.enable_lineage_history ? 1 : 0
  project      = var.project_id
  subscription = google_pubsub_subscription.field_edges_bq[0].name
  role         = "roles/pubsub.subscriber"
  member       = "serviceAccount:service-${var.project_number != "" ? var.project_number : data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_pubsub_subscription" "field_edges_dlq_pull" {
  count   = var.enable_lineage_history ? 1 : 0
  project = var.project_id
  name    = "${var.field_edges_topic}-dlq-sub${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  topic   = google_pubsub_topic.field_edges_dlq[0].name

  message_retention_duration = "604800s"
  retain_acked_messages      = false

  expiration_policy {
    ttl = ""
  }
}

resource "google_bigquery_table" "v_field_closures" {
  count               = var.enable_lineage_history ? 1 : 0
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.supercargo_catalog[0].dataset_id
  table_id            = var.field_closures_view_id
  deletion_protection = false

  view {
    query          = <<EOF
WITH RECURSIVE
ranked_edges AS (
  SELECT
    from_field_urn,
    to_field_urn,
    transformation_kind,
    COALESCE(approved_mask, FALSE) AS approved_mask,
    confidence,
    updated_at,
    last_observed_at,
    deleted_at,
    ROW_NUMBER() OVER (
      PARTITION BY from_field_urn, to_field_urn, COALESCE(source_system, '')
      ORDER BY updated_at DESC, last_observed_at DESC
    ) AS rn
  FROM `${var.project_id}.${google_bigquery_dataset.supercargo_catalog[0].dataset_id}.${google_bigquery_table.field_edges[0].table_id}`
),

active_edges AS (
  SELECT
    from_field_urn,
    to_field_urn,
    transformation_kind,
    approved_mask,
    confidence
  FROM ranked_edges
  WHERE rn = 1
    AND deleted_at IS NULL
),

root_fields AS (
  SELECT from_field_urn AS field_urn FROM active_edges
  UNION DISTINCT
  SELECT to_field_urn AS field_urn FROM active_edges
),

upstream_walk AS (
  SELECT
    r.field_urn AS root_field_urn,
    r.field_urn AS current_field_urn,
    CAST(NULL AS STRING) AS ancestor_field_urn,
    0 AS depth,
    CONCAT('|', r.field_urn, '|') AS path,
    0 AS confidence_rank
  FROM root_fields r
  UNION ALL
  SELECT
    w.root_field_urn,
    e.from_field_urn AS current_field_urn,
    e.from_field_urn AS ancestor_field_urn,
    w.depth + 1 AS depth,
    CONCAT(w.path, e.from_field_urn, '|') AS path,
    CASE UPPER(COALESCE(e.confidence, ''))
      WHEN 'OBSERVED' THEN 3
      WHEN 'DECLARED' THEN 2
      WHEN 'INFERRED' THEN 1
      ELSE 0
    END AS confidence_rank
  FROM upstream_walk w
  JOIN active_edges e
    ON e.to_field_urn = w.current_field_urn
  WHERE w.depth < 10
    AND STRPOS(w.path, CONCAT('|', e.from_field_urn, '|')) = 0
),

downstream_walk AS (
  SELECT
    r.field_urn AS root_field_urn,
    r.field_urn AS current_field_urn,
    CAST(NULL AS STRING) AS descendant_field_urn,
    0 AS depth,
    CONCAT('|', r.field_urn, '|') AS path,
    FALSE AS path_masked,
    0 AS confidence_rank
  FROM root_fields r
  UNION ALL
  SELECT
    w.root_field_urn,
    e.to_field_urn AS current_field_urn,
    e.to_field_urn AS descendant_field_urn,
    w.depth + 1 AS depth,
    CONCAT(w.path, e.to_field_urn, '|') AS path,
    (w.path_masked OR e.approved_mask) AS path_masked,
    CASE UPPER(COALESCE(e.confidence, ''))
      WHEN 'OBSERVED' THEN 3
      WHEN 'DECLARED' THEN 2
      WHEN 'INFERRED' THEN 1
      ELSE 0
    END AS confidence_rank
  FROM downstream_walk w
  JOIN active_edges e
    ON e.from_field_urn = w.current_field_urn
  WHERE w.depth < 10
    AND STRPOS(w.path, CONCAT('|', e.to_field_urn, '|')) = 0
),

upstream_agg AS (
  SELECT
    root_field_urn,
    IFNULL(ARRAY_AGG(DISTINCT IF(depth > 0, ancestor_field_urn, NULL) IGNORE NULLS), CAST([] AS ARRAY<STRING>)) AS upstream,
    COALESCE(LOGICAL_OR(depth >= 10), FALSE) AS upstream_truncated,
    MAX(confidence_rank) AS max_confidence_rank
  FROM upstream_walk
  GROUP BY root_field_urn
),

downstream_agg AS (
  SELECT
    root_field_urn,
    IFNULL(ARRAY_AGG(DISTINCT IF(depth > 0, descendant_field_urn, NULL) IGNORE NULLS), CAST([] AS ARRAY<STRING>)) AS downstream,
    COALESCE(LOGICAL_OR(depth >= 10), FALSE) AS downstream_truncated,
    COALESCE(LOGICAL_OR(depth > 0 AND NOT path_masked), FALSE) AS has_unmasked_downstream,
    MAX(confidence_rank) AS max_confidence_rank
  FROM downstream_walk
  GROUP BY root_field_urn
)

SELECT
  r.field_urn,
  COALESCE(ua.upstream, CAST([] AS ARRAY<STRING>)) AS upstream,
  COALESCE(da.downstream, CAST([] AS ARRAY<STRING>)) AS downstream,
  COALESCE(ua.upstream_truncated, FALSE) AS upstream_truncated,
  COALESCE(da.downstream_truncated, FALSE) AS downstream_truncated,
  COALESCE(da.has_unmasked_downstream, FALSE) AS has_unmasked_downstream,
  CASE GREATEST(COALESCE(ua.max_confidence_rank, 0), COALESCE(da.max_confidence_rank, 0))
    WHEN 3 THEN 'OBSERVED'
    WHEN 2 THEN 'DECLARED'
    WHEN 1 THEN 'INFERRED'
    ELSE CAST(NULL AS STRING)
  END AS max_confidence,
  CURRENT_TIMESTAMP() AS computed_at
FROM root_fields r
LEFT JOIN upstream_agg ua
  ON r.field_urn = ua.root_field_urn
LEFT JOIN downstream_agg da
  ON r.field_urn = da.root_field_urn
EOF
    use_legacy_sql = false
  }
}

resource "google_bigquery_dataset_iam_member" "hub_catalog_bq_viewer" {
  count      = var.enable_lineage_history ? 1 : 0
  project    = var.project_id
  dataset_id = google_bigquery_dataset.supercargo_catalog[0].dataset_id
  role       = "roles/bigquery.dataViewer"
  member     = "serviceAccount:${google_service_account.hub_runtime.email}"
}

resource "google_project_iam_member" "hub_bq_job_user" {
  count   = var.enable_lineage_history ? 1 : 0
  project = var.project_id
  role    = "roles/bigquery.jobUser"
  member  = "serviceAccount:${google_service_account.hub_runtime.email}"
}

# --- Event-Driven Alert Relay Infrastructure ---

resource "google_pubsub_topic" "events" {
  project = var.project_id
  name    = "supercargo-events${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
}

resource "google_pubsub_topic" "contract_changed" {
  project = var.project_id
  name    = "catalog-contract-changed${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
}

resource "google_pubsub_topic" "contract_deleted" {
  project = var.project_id
  name    = "catalog-contract-deleted${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
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
  account_id   = "events-invoker-sa${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  display_name = "Hub Alert Relay Push Invoker"
}

resource "google_cloud_run_v2_service_iam_member" "events_invoker_run" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.hub.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.events_invoker.email}"
}

resource "google_service_account_iam_member" "pubsub_events_invoker_token_creator" {
  service_account_id = google_service_account.events_invoker.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:service-${var.project_number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_pubsub_subscription" "events_push" {
  project = var.project_id
  name    = "hub-events-push${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  topic   = google_pubsub_topic.events.name

  ack_deadline_seconds = 60

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.hub.uri}/v1/internal/events"
    oidc_token {
      service_account_email = google_service_account.events_invoker.email
      audience              = var.oidc_audience != "" ? trimspace(split(",", var.oidc_audience)[0]) : google_cloud_run_v2_service.hub.uri
    }
  }

  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }

  expiration_policy {
    ttl = ""
  }

  depends_on = [
    google_cloud_run_v2_service_iam_member.events_invoker_run,
    google_service_account_iam_member.pubsub_events_invoker_token_creator
  ]
}

# ----------------------------------------------------------------------------------------------------------------------
# MCP COMPANION SERVICE (HTTP/SSE Cloud Run Sidecar)
# ----------------------------------------------------------------------------------------------------------------------

resource "google_service_account" "mcp_runtime" {
  count        = var.mcp_enabled ? 1 : 0
  project      = var.project_id
  account_id   = "supercargo-mcp-sa${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  display_name = "Supercargo MCP Companion Runtime Service Account"
  depends_on   = [time_sleep.wait_for_apis]
}

resource "google_cloud_run_v2_service_iam_member" "mcp_hub_invoker" {
  count    = var.mcp_enabled ? 1 : 0
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.hub.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.mcp_runtime[0].email}"
}

resource "google_cloud_run_v2_service" "mcp" {
  count               = var.mcp_enabled ? 1 : 0
  provider            = google-beta
  project             = var.project_id
  name                = "supercargo-mcp${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  location            = var.region
  ingress             = var.ingress_type
  custom_audiences    = var.mcp_oidc_audience != "" ? [var.mcp_oidc_audience] : (var.oidc_audience != "" ? [var.oidc_audience] : [])
  deletion_protection = false

  depends_on = [time_sleep.wait_for_apis]

  template {
    service_account  = google_service_account.mcp_runtime[0].email
    session_affinity = true
    timeout          = "3600s"

    scaling {
      min_instance_count = 0
      max_instance_count = var.mcp_max_instances
    }

    containers {
      image = "${var.region}-docker.pkg.dev/${var.image_project_id}/hub/supercargo-mcp:${var.mcp_image_tag == "" ? "latest" : var.mcp_image_tag}"

      ports {
        container_port = 8085
        name           = "h2c"
      }

      resources {
        cpu_idle = true
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
      }

      env {
        name  = "HUB_URL"
        value = google_cloud_run_v2_service.hub.uri
      }

      env {
        name  = "ENVIRONMENT"
        value = var.environment
      }

      env {
        name  = "GCP_PROJECT_ID"
        value = var.firestore_project_id
      }

      env {
        name  = "IAM_DOMAIN"
        value = var.iam_domain
      }

      env {
        name  = "OIDC_AUDIENCE"
        value = var.mcp_oidc_audience != "" ? var.mcp_oidc_audience : var.oidc_audience
      }

      env {
        name  = "HUB_OIDC_AUDIENCE"
        value = var.oidc_audience != "" ? trimspace(split(",", var.oidc_audience)[0]) : google_cloud_run_v2_service.hub.uri
      }

      env {
        name  = "AUTH_ENFORCE"
        value = var.auth_enforce ? "true" : "false"
      }

      env {
        name  = "FORCE_DEPLOY"
        value = var.force_deploy_trigger
      }

      startup_probe {
        http_get {
          path = "/healthz"
          port = 8085
        }
        initial_delay_seconds = 0
        period_seconds        = 5
        timeout_seconds       = 3
        failure_threshold     = 12
      }

      liveness_probe {
        http_get {
          path = "/healthz"
          port = 8085
        }
        period_seconds    = 10
        timeout_seconds   = 3
        failure_threshold = 3
      }
    }
  }

  traffic {
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
    percent = 100
  }

  lifecycle {
    ignore_changes = [
      template[0].containers[0].image,
      client,
      client_version
    ]
  }
}

resource "google_cloud_run_v2_service_iam_member" "mcp_invokers" {
  for_each = var.mcp_enabled ? toset(var.mcp_allowed_invokers) : []
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.mcp[0].name
  role     = "roles/run.invoker"
  member   = each.value
}

# ----------------------------------------------------------------------------------------------------------------------
# CONSOLE COMPANION SERVICE (SSR Web UI Sidecar)
# ----------------------------------------------------------------------------------------------------------------------

resource "google_service_account" "console_runtime" {
  count        = var.console_enabled ? 1 : 0
  project      = var.project_id
  account_id   = "console-runtime-sa${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  display_name = "Supercargo Console Companion Runtime Service Account"
  depends_on   = [time_sleep.wait_for_apis]
}

resource "google_cloud_run_v2_service_iam_member" "console_hub_invoker" {
  count    = var.console_enabled ? 1 : 0
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.hub.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.console_runtime[0].email}"
}

resource "google_cloud_run_v2_service" "console" {
  count               = var.console_enabled ? 1 : 0
  provider            = google-beta
  project             = var.project_id
  name                = "console${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  location            = var.region
  launch_stage        = "BETA"
  ingress             = var.ingress_type
  iap_enabled         = var.console_iap_enabled
  deletion_protection = false

  depends_on = [time_sleep.wait_for_apis]

  template {
    service_account = google_service_account.console_runtime[0].email

    scaling {
      min_instance_count = 0
      max_instance_count = var.console_max_instances
    }

    containers {
      image = "${var.region}-docker.pkg.dev/${var.image_project_id}/hub/supercargo-console:${var.console_image_tag == "" ? "latest" : var.console_image_tag}"

      ports {
        container_port = 8088
      }

      resources {
        cpu_idle = true
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
      }

      env {
        name  = "HUB_URL"
        value = google_cloud_run_v2_service.hub.uri
      }

      env {
        name  = "HUB_OIDC_AUDIENCE"
        value = var.oidc_audience != "" ? trimspace(split(",", var.oidc_audience)[0]) : google_cloud_run_v2_service.hub.uri
      }

      env {
        name  = "ENVIRONMENT"
        value = var.environment
      }

      env {
        name  = "SUPERCARGO_ENV"
        value = var.environment
      }

      env {
        name  = "GCP_PROJECT_ID"
        value = var.firestore_project_id
      }

      env {
        name  = "IAP_ENFORCE"
        value = var.console_iap_enabled ? "true" : "false"
      }

      env {
        name  = "IAP_AUDIENCE"
        value = local.console_iap_audience
      }

      env {
        name  = "TELEMETRY_MASTER_SALT"
        value = local.console_telemetry_identity_salt
      }

      env {
        name  = "FORCE_DEPLOY"
        value = var.force_deploy_trigger
      }

      startup_probe {
        http_get {
          path = "/healthz"
          port = 8088
        }
        initial_delay_seconds = 0
        period_seconds        = 5
        timeout_seconds       = 2
        failure_threshold     = 12
      }

      liveness_probe {
        http_get {
          path = "/healthz"
          port = 8088
        }
        period_seconds    = 10
        timeout_seconds   = 2
        failure_threshold = 3
      }
    }
  }

  traffic {
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
    percent = 100
  }

  lifecycle {
    ignore_changes = [
      template[0].containers[0].image,
      client,
      client_version
    ]
  }
}

resource "google_cloud_run_v2_service_iam_member" "console_invokers" {
  for_each = var.console_enabled ? toset(var.console_allowed_invokers) : []
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.console[0].name
  role     = "roles/run.invoker"
  member   = each.value
}

# When Direct IAP is enabled on the Console companion service, the Cloud Run IAP service agent must have roles/run.invoker
resource "google_cloud_run_v2_service_iam_member" "console_iap_service_agent_invoker" {
  count    = var.console_enabled && var.console_iap_enabled ? 1 : 0
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.console[0].name
  role     = "roles/run.invoker"
  member   = "serviceAccount:service-${var.project_number}@gcp-sa-iap.iam.gserviceaccount.com"
}

# Grant roles/iap.httpsResourceAccessor to authorized members when Direct IAP is enabled
resource "google_iap_web_cloud_run_service_iam_member" "console_iap_accessors" {
  for_each               = var.console_enabled && var.console_iap_enabled ? toset(var.console_iap_allowed_members) : toset([])
  project                = var.project_id
  location               = var.region
  cloud_run_service_name = google_cloud_run_v2_service.console[0].name
  role                   = "roles/iap.httpsResourceAccessor"
  member                 = each.value
}


# ----------------------------------------------------------------------------------------------------------------------
# DATAFORM CLOUD LOGGING SINK & EVENT INGESTION INFRASTRUCTURE
# ----------------------------------------------------------------------------------------------------------------------

resource "google_pubsub_topic" "dataform_events" {
  count   = var.enable_dataform_integration ? 1 : 0
  project = var.project_id
  name    = var.dataform_events_topic
}

resource "google_pubsub_topic" "dataform_events_dlq" {
  count   = var.enable_dataform_integration ? 1 : 0
  project = var.project_id
  name    = "${var.dataform_events_topic}-dlq"
}

resource "google_logging_project_sink" "dataform_events" {
  count                  = var.enable_dataform_integration ? 1 : 0
  project                = var.project_id
  name                   = "supercargo-dataform-events-sink${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  destination            = "pubsub.googleapis.com/${google_pubsub_topic.dataform_events[0].id}"
  filter                 = var.dataform_sink_filter != "" ? var.dataform_sink_filter : local.default_dataform_sink_filter
  unique_writer_identity = true
}

resource "google_pubsub_topic_iam_member" "dataform_sink_publisher" {
  count   = var.enable_dataform_integration ? 1 : 0
  project = var.project_id
  topic   = google_pubsub_topic.dataform_events[0].name
  role    = "roles/pubsub.publisher"
  member  = google_logging_project_sink.dataform_events[0].writer_identity
}

resource "google_pubsub_subscription" "dataform_events_push" {
  count   = var.enable_dataform_integration ? 1 : 0
  project = var.project_id
  name    = "hub-dataform-events-push${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  topic   = google_pubsub_topic.dataform_events[0].name

  ack_deadline_seconds = 60

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.hub.uri}/v1/internal/dataform-events"
    oidc_token {
      service_account_email = google_service_account.events_invoker.email
      audience              = var.oidc_audience != "" ? trimspace(split(",", var.oidc_audience)[0]) : google_cloud_run_v2_service.hub.uri
    }
  }

  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.dataform_events_dlq[0].id
    max_delivery_attempts = 5
  }

  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }

  expiration_policy {
    ttl = ""
  }

  depends_on = [
    google_cloud_run_v2_service_iam_member.events_invoker_run,
    google_service_account_iam_member.pubsub_events_invoker_token_creator,
    google_pubsub_topic_iam_member.dataform_dlq_publisher,
  ]
}

resource "google_pubsub_topic_iam_member" "dataform_dlq_publisher" {
  count   = var.enable_dataform_integration ? 1 : 0
  project = var.project_id
  topic   = google_pubsub_topic.dataform_events_dlq[0].name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:service-${var.project_number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_pubsub_subscription_iam_member" "dataform_push_dlq_subscriber" {
  count        = var.enable_dataform_integration ? 1 : 0
  project      = var.project_id
  subscription = google_pubsub_subscription.dataform_events_push[0].name
  role         = "roles/pubsub.subscriber"
  member       = "serviceAccount:service-${var.project_number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_pubsub_subscription" "dataform_events_dlq_pull" {
  count   = var.enable_dataform_integration ? 1 : 0
  project = var.project_id
  name    = "${var.dataform_events_topic}-dlq-sub"
  topic   = google_pubsub_topic.dataform_events_dlq[0].name

  message_retention_duration = "604800s"
  retain_acked_messages      = false

  expiration_policy {
    ttl = ""
  }
}

# --- Cloud Monitoring Pack Integration ---

module "monitoring" {
  count  = var.enable_monitoring ? 1 : 0
  source = "../monitoring_control_plane"

  project_id  = var.project_id
  environment = var.environment
  name_suffix = var.name_suffix
  region      = var.region

  # Service Identifiers
  hub_service_name                   = google_cloud_run_v2_service.hub.name
  vault_service_name                 = ""
  shovel_service_name                = google_cloud_run_v2_service.shovel.name
  mcp_service_name                   = var.mcp_enabled ? google_cloud_run_v2_service.mcp[0].name : ""
  console_service_name               = var.console_enabled ? google_cloud_run_v2_service.console[0].name : ""
  analytics_dashboard_service_name   = ""
  github_runner_webhook_service_name = ""

  # Service Max Instances
  hub_max_instances     = var.max_instances
  shovel_max_instances  = var.shovel_max_instances
  mcp_max_instances     = var.mcp_max_instances
  console_max_instances = var.console_max_instances

  # Storage & CDC Identifiers
  shovel_trigger_name                    = google_eventarc_trigger.outbox_trigger.name
  firestore_database_id                  = google_firestore_database.database.name
  outbox_audit_subscription_name         = var.enable_audit_sink ? google_pubsub_subscription.outbox_audit_bq[0].name : ""
  health_events_subscription_name        = var.enable_health_mesh ? google_pubsub_subscription.health_events_bq[0].name : ""
  events_push_subscription_name          = ""
  dataform_events_push_subscription_name = var.enable_dataform_integration ? google_pubsub_subscription.dataform_events_push[0].name : ""
  dataform_events_dlq_subscription_name  = var.enable_dataform_integration ? google_pubsub_subscription.dataform_events_dlq_pull[0].name : ""

  # Batch Jobs & Meta-Telemetry (Empty in hub standalone deployment)
  monitored_batch_jobs    = []
  telemetry_subscriptions = []

  # SLA Tiering & Alert Routing
  sla_tier                             = var.sla_tier
  enable_control_plane_alerts          = true
  enable_slos                          = true
  enable_dashboards                    = true
  alert_slack_channel                  = var.alert_slack_channel
  alert_email_address                  = var.alert_email_address
  alert_pagerduty_service_key          = var.alert_pagerduty_service_key
  alert_notification_channels          = var.alert_notification_channels
  additional_sre_notification_channels = var.additional_sre_notification_channels
}

# ----------------------------------------------------------------------------------------------------------------------
# CLOUD RUN JOB LOGGING SINK, PUB/SUB & CLOUD MONITORING ALERT INTEGRATION
# ----------------------------------------------------------------------------------------------------------------------

resource "google_pubsub_topic" "job_events" {
  count   = var.enable_job_monitoring_integration ? 1 : 0
  project = var.project_id
  name    = var.job_events_topic
}

resource "google_pubsub_topic" "job_events_dlq" {
  count   = var.enable_job_monitoring_integration ? 1 : 0
  project = var.project_id
  name    = "${var.job_events_topic}-dlq"
}

resource "google_logging_project_sink" "job_events" {
  count                  = var.enable_job_monitoring_integration ? 1 : 0
  project                = var.project_id
  name                   = "supercargo-job-events-sink${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  destination            = "pubsub.googleapis.com/${google_pubsub_topic.job_events[0].id}"
  filter                 = var.job_sink_filter != "" ? var.job_sink_filter : local.default_job_sink_filter
  unique_writer_identity = true
}

resource "google_pubsub_topic_iam_member" "job_sink_publisher" {
  count   = var.enable_job_monitoring_integration ? 1 : 0
  project = var.project_id
  topic   = google_pubsub_topic.job_events[0].name
  role    = "roles/pubsub.publisher"
  member  = google_logging_project_sink.job_events[0].writer_identity
}

resource "google_monitoring_notification_channel" "hub_events" {
  count        = var.enable_job_monitoring_integration ? 1 : 0
  project      = var.project_id
  display_name = "Supercargo Hub Events Notification Channel${var.name_suffix != "" ? " (${var.name_suffix})" : ""}"
  type         = "pubsub"
  labels = {
    topic = google_pubsub_topic.job_events[0].id
  }
}

resource "google_pubsub_topic_iam_member" "monitoring_notification_publisher" {
  count   = var.enable_job_monitoring_integration ? 1 : 0
  project = var.project_id
  topic   = google_pubsub_topic.job_events[0].name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:service-${var.project_number != "" ? var.project_number : data.google_project.project.number}@gcp-sa-monitoring-notification.iam.gserviceaccount.com"
}

resource "google_pubsub_subscription" "job_events_push" {
  count   = var.enable_job_monitoring_integration ? 1 : 0
  project = var.project_id
  name    = "hub-job-events-push${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  topic   = google_pubsub_topic.job_events[0].name

  ack_deadline_seconds = 60

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.hub.uri}/v1/internal/monitoring-events"
    oidc_token {
      service_account_email = google_service_account.events_invoker.email
      audience              = var.oidc_audience != "" ? trimspace(split(",", var.oidc_audience)[0]) : google_cloud_run_v2_service.hub.uri
    }
  }

  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.job_events_dlq[0].id
    max_delivery_attempts = 5
  }

  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }

  expiration_policy {
    ttl = ""
  }

  depends_on = [
    google_cloud_run_v2_service_iam_member.events_invoker_run,
    google_service_account_iam_member.pubsub_events_invoker_token_creator,
    google_pubsub_topic_iam_member.job_dlq_publisher,
  ]
}

resource "google_pubsub_topic_iam_member" "job_dlq_publisher" {
  count   = var.enable_job_monitoring_integration ? 1 : 0
  project = var.project_id
  topic   = google_pubsub_topic.job_events_dlq[0].name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:service-${var.project_number != "" ? var.project_number : data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_pubsub_subscription_iam_member" "job_push_dlq_subscriber" {
  count        = var.enable_job_monitoring_integration ? 1 : 0
  project      = var.project_id
  subscription = google_pubsub_subscription.job_events_push[0].name
  role         = "roles/pubsub.subscriber"
  member       = "serviceAccount:service-${var.project_number != "" ? var.project_number : data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_pubsub_subscription" "job_events_dlq_pull" {
  count   = var.enable_job_monitoring_integration ? 1 : 0
  project = var.project_id
  name    = "${var.job_events_topic}-dlq-sub"
  topic   = google_pubsub_topic.job_events_dlq[0].name

  message_retention_duration = "604800s"
  retain_acked_messages      = false

  expiration_policy {
    ttl = ""
  }
}
