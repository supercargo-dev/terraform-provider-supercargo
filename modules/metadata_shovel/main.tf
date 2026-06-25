resource "google_service_account" "shovel" {
  account_id   = "${var.service_name}-sa"
  display_name = "Metadata Shovel Service Account"
  project      = var.project_id
}

resource "google_cloud_run_v2_service" "shovel" {
  name     = var.service_name
  location = var.region
  project  = var.project_id

  ingress = var.ingress_type

  deletion_protection = false

  template {
    service_account = google_service_account.shovel.email
    containers {
      image = var.image
      env {
        name  = "GCP_PROJECT_ID"
        value = var.project_id
      }
      env {
        name  = "FIRESTORE_DATABASE"
        value = var.firestore_database
      }
      env {
        name  = "TARGET_TOPIC"
        value = var.default_topic_id
      }
      env {
        name  = "TOPIC_ROUTING"
        value = jsonencode(var.topic_routing)
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

resource "google_cloud_run_v2_service_iam_member" "shovel_public_access" {
  count    = var.ingress_type == "INGRESS_TRAFFIC_ALL" ? 1 : 0
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.shovel.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

resource "google_project_iam_member" "shovel_firestore" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.shovel.email}"
}

resource "google_pubsub_topic_iam_member" "shovel_publisher_default" {
  topic   = var.default_topic_id
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.shovel.email}"
  project = var.project_id
}

resource "google_pubsub_topic_iam_member" "shovel_publisher_routed" {
  for_each = var.topic_routing
  topic    = each.value
  role     = "roles/pubsub.publisher"
  member   = "serviceAccount:${google_service_account.shovel.email}"
  project  = var.project_id
}
