locals {
  normalized_licensed_domains = var.iap_enabled ? toset([
    for d in var.licensed_domains : startswith(d, "domain:") ? d : "domain:${d}"
  ]) : toset([])
}

data "google_project" "project" {
  project_id = var.project_id
}

resource "google_cloud_run_v2_service" "docs_portal" {
  name     = var.service_name
  location = var.region
  project  = var.project_id

  ingress      = "INGRESS_TRAFFIC_ALL"
  launch_stage = "BETA"
  iap_enabled  = var.iap_enabled

  deletion_protection = false

  template {
    scaling {
      min_instance_count = 0
      max_instance_count = var.max_instances
    }

    containers {
      image = var.image

      resources {
        cpu_idle = true
        limits = {
          memory = "256Mi"
          cpu    = "1"
        }
      }

      ports {
        container_port = 8080
      }

      startup_probe {
        http_get {
          path = "/healthz"
          port = 8080
        }
      }

      liveness_probe {
        http_get {
          path = "/healthz"
          port = 8080
        }
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
      client_version,
    ]
  }
}

# When IAP is enabled, the IAP service agent must have roles/run.invoker to invoke Cloud Run.
resource "google_cloud_run_v2_service_iam_member" "iap_service_agent_invoker" {
  count = var.iap_enabled ? 1 : 0

  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.docs_portal.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:service-${data.google_project.project.number}@gcp-sa-iap.iam.gserviceaccount.com"
}

# When IAP is disabled (public docs), allUsers has roles/run.invoker.
resource "google_cloud_run_v2_service_iam_member" "public_invoker" {
  count = var.iap_enabled ? 0 : 1

  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.docs_portal.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# Grant IAP-secured Web App User role to licensed customer domains on the Cloud Run service.
resource "google_iap_web_cloud_run_service_iam_member" "licensed_domains" {
  for_each = local.normalized_licensed_domains

  project                = var.project_id
  location               = var.region
  cloud_run_service_name = google_cloud_run_v2_service.docs_portal.name
  role                   = "roles/iap.httpsResourceAccessor"
  member                 = each.value
}

# Map custom domain directly to Cloud Run service without Load Balancer ($0 base cost).
resource "google_cloud_run_domain_mapping" "docs_portal" {
  count = var.domain_name != "" ? 1 : 0

  location = var.region
  name     = var.domain_name

  metadata {
    namespace = var.project_id
  }

  spec {
    route_name = google_cloud_run_v2_service.docs_portal.name
  }
}
