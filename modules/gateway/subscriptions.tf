// Service Account for Pub/Sub to invoke Cloud Run
resource "google_service_account" "push_invoker" {
  account_id   = "push-invoker-${var.product_id}-${random_id.suffix.hex}"
  display_name = "Push Invoker for ${var.product_id}"
  project      = var.project_id
}

// Grant Invoker role to the SA
resource "google_cloud_run_v2_service_iam_member" "invoker" {
  name     = google_cloud_run_v2_service.gateway.name
  location = google_cloud_run_v2_service.gateway.location
  project  = var.project_id
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.push_invoker.email}"
}

// Allow Pub/Sub to use the service account to create tokens
resource "google_service_account_iam_member" "push_invoker_token_creator" {
  service_account_id = google_service_account.push_invoker.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:service-${data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

// IAM: Pub/Sub Service Agent needs permission to publish to the DLQ topic
resource "google_pubsub_topic_iam_member" "pubsub_dlq_publisher" {
  project = var.project_id
  topic   = google_pubsub_topic.dlq.name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:service-${data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

// IAM: Pub/Sub Service Agent needs permission to acknowledge messages on the subscription
resource "google_pubsub_subscription_iam_member" "pubsub_dlq_subscriber" {
  project      = var.project_id
  subscription = google_pubsub_subscription.raw_push.name
  role         = "roles/pubsub.subscriber"
  member       = "serviceAccount:service-${data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

// Raw Subscription (Push to Guardian)
resource "google_pubsub_subscription" "raw_push" {
  name    = "raw-sub-${var.product_id}-${random_id.suffix.hex}"
  topic   = google_pubsub_topic.raw.name
  project = var.project_id

  depends_on = [google_cloud_run_v2_service_iam_member.invoker]

  message_retention_duration = var.pubsub_message_retention_duration
  expiration_policy {
    ttl = ""
  }

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.gateway.uri}${var.pubsub_push_path}"
    oidc_token {
      service_account_email = google_service_account.push_invoker.email
      audience              = var.gateway_audience
    }
  }

  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.dlq.id
    max_delivery_attempts = var.pubsub_max_delivery_attempts
  }

  retry_policy {
    minimum_backoff = var.pubsub_minimum_backoff
    maximum_backoff = var.pubsub_maximum_backoff
  }
}

// Clean Subscription (Pull) - for consumers
resource "google_pubsub_subscription" "clean_sub" {
  name    = "clean-sub-${var.product_id}-${random_id.suffix.hex}"
  topic   = google_pubsub_topic.clean.name
  project = var.project_id

  message_retention_duration = var.pubsub_message_retention_duration
  expiration_policy {
    ttl = var.pubsub_expiration_policy_ttl
  }
}

// Clean Subscription (BigQuery) - for persistence
resource "google_pubsub_subscription" "clean_bq" {
  for_each = var.contracts
  name     = "clean-bq-${var.product_id}-${replace(each.key, "-", "_")}-${random_id.suffix.hex}"
  topic    = google_pubsub_topic.clean.name
  project  = var.project_id

  message_retention_duration = var.pubsub_message_retention_duration
  expiration_policy {
    ttl = ""
  }

  # Filter: Only ingest messages for this specific contract
  filter = "attributes.urn = \"${each.value.id}\""

  bigquery_config {
    table               = "${var.project_id}.${var.bigquery_dataset_id}.${google_bigquery_table.validated_data[each.key].table_id}"
    use_topic_schema    = false
    use_table_schema    = var.bigquery_use_table_schema
    write_metadata      = false
    drop_unknown_fields = true
  }

  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.dlq.id
    max_delivery_attempts = var.pubsub_max_delivery_attempts
  }

  retry_policy {
    minimum_backoff = var.pubsub_minimum_backoff
    maximum_backoff = var.pubsub_maximum_backoff
  }

  depends_on = [google_project_iam_member.pubsub_bq_writer]
}

// IAM: Pub/Sub Service Agent needs permission to acknowledge messages on the clean_bq subscription for DLQ routing
resource "google_pubsub_subscription_iam_member" "pubsub_clean_bq_dlq_subscriber" {
  for_each     = var.contracts
  project      = var.project_id
  subscription = google_pubsub_subscription.clean_bq[each.key].name
  role         = "roles/pubsub.subscriber"
  member       = "serviceAccount:service-${data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

// IAM: Pub/Sub needs permission to write to BigQuery
resource "google_bigquery_dataset_iam_member" "pubsub_bq_writer" {
  count      = length(var.contracts) > 0 ? 1 : 0
  project    = var.project_id
  dataset_id = var.bigquery_dataset_id
  role       = "roles/bigquery.dataEditor"
  member     = "serviceAccount:service-${data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_bigquery_dataset_iam_member" "pubsub_bq_metadata" {
  count      = length(var.contracts) > 0 ? 1 : 0
  project    = var.project_id
  dataset_id = var.bigquery_dataset_id
  role       = "roles/bigquery.metadataViewer"
  member     = "serviceAccount:service-${data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

data "google_project" "project" {
  project_id = var.project_id
}

// DLQ Subscription (Pull) - for inspection
resource "google_pubsub_subscription" "dlq_sub" {
  name    = "dlq-sub-${var.product_id}-${random_id.suffix.hex}"
  topic   = google_pubsub_topic.dlq.name
  project = var.project_id

  message_retention_duration = var.pubsub_message_retention_duration
  expiration_policy {
    ttl = var.pubsub_expiration_policy_ttl
  }
}
