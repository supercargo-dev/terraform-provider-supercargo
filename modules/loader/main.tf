resource "google_service_account" "loader_sa" {
  account_id   = "loader-${var.domain_name}"
  display_name = "Loader Service Account for ${var.domain_name}"
  project      = var.project_id
}

resource "google_cloud_run_v2_service" "loader" {
  name     = "loader-${var.domain_name}"
  location = var.region
  project  = var.project_id

  ingress = var.ingress_type

  deletion_protection = false

  template {
    service_account = google_service_account.loader_sa.email
    containers {
      image = var.image
      env {
        name  = "INGRESS_CONFIG"
        value = var.ingress_config
      }
      env {
        name  = "PROJECT_ID"
        value = var.project_id
      }
      env {
        name  = "DLQ_TOPIC"
        value = var.dlq_topic_id
      }
      resources {
        limits = {
          memory = "512Mi"
          cpu    = "1"
        }
      }
    }
  }
}

resource "google_cloud_run_v2_service_iam_member" "loader_public_access" {
  count    = var.ingress_type == "INGRESS_TRAFFIC_ALL" ? 1 : 0
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.loader.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

// IAM for Service
resource "google_pubsub_topic_iam_member" "target_publisher" {
  for_each = toset(concat(var.target_topic_ids, [var.dlq_topic_id]))
  project  = var.project_id
  topic    = each.key
  role     = "roles/pubsub.publisher"
  member   = "serviceAccount:${google_service_account.loader_sa.email}"
}

resource "google_storage_bucket_iam_member" "viewer" {
  bucket = var.bucket_name
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${google_service_account.loader_sa.email}"
}

// Trigger Mechanism
resource "google_pubsub_topic" "trigger" {
  name    = "gcs-trigger-${var.domain_name}"
  project = var.project_id
}

// Grant GCS permission to publish to trigger topic
data "google_storage_project_service_account" "gcs_account" {
  project = var.project_id
}

resource "google_pubsub_topic_iam_member" "gcs_publisher" {
  topic   = google_pubsub_topic.trigger.name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${data.google_storage_project_service_account.gcs_account.email_address}"
  project = var.project_id
}

resource "google_storage_notification" "notification" {
  bucket         = var.bucket_name
  payload_format = "JSON_API_V1"
  topic          = google_pubsub_topic.trigger.id
  event_types    = ["OBJECT_FINALIZE"]

  depends_on = [google_pubsub_topic_iam_member.gcs_publisher]
}

// Push Subscription Invoker Identity
resource "google_service_account" "invoker" {
  account_id   = "trigger-invoker-${var.domain_name}"
  display_name = "Invoker SA for ${var.domain_name}"
  project      = var.project_id
}

resource "google_cloud_run_v2_service_iam_member" "invoker" {
  name     = google_cloud_run_v2_service.loader.name
  location = google_cloud_run_v2_service.loader.location
  project  = var.project_id
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.invoker.email}"
}

data "google_project" "project" {
  project_id = var.project_id
}

// IAM: Pub/Sub Service Agent needs permission to publish to the DLQ topic
resource "google_pubsub_topic_iam_member" "pubsub_dlq_publisher" {
  project = var.project_id
  topic   = var.dlq_topic_id
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:service-${data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

// IAM: Pub/Sub Service Agent needs permission to acknowledge messages on the subscription
resource "google_pubsub_subscription_iam_member" "pubsub_dlq_subscriber" {
  project      = var.project_id
  subscription = google_pubsub_subscription.subscription.name
  role         = "roles/pubsub.subscriber"
  member       = "serviceAccount:service-${data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_pubsub_subscription" "subscription" {
  name    = "loader-sub-${var.domain_name}"
  topic   = google_pubsub_topic.trigger.name
  project = var.project_id

  message_retention_duration = "86400s"
  expiration_policy {
    ttl = ""
  }

  push_config {
    push_endpoint = google_cloud_run_v2_service.loader.uri
    oidc_token {
      service_account_email = google_service_account.invoker.email
    }
  }

  dead_letter_policy {
    dead_letter_topic     = var.dlq_topic_id
    max_delivery_attempts = 5
  }

  retry_policy {
    minimum_backoff = "30s"
    maximum_backoff = "600s"
  }

  ack_deadline_seconds = 600
}
