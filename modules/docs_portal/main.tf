locals {
  normalized_licensed_domains = var.iap_enabled && var.iap_client_id != "" ? toset([
    for d in var.licensed_domains : startswith(d, "domain:") ? d : "domain:${d}"
  ]) : toset([])
}

resource "google_cloud_run_v2_service" "docs_portal" {
  name     = var.service_name
  location = var.region
  project  = var.project_id

  ingress             = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"
  deletion_protection = false

  template {
    containers {
      image = var.image

      resources {
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
}

resource "google_cloud_run_v2_service_iam_member" "all_users_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.docs_portal.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

resource "google_compute_region_network_endpoint_group" "serverless_neg" {
  name                  = "${var.service_name}-neg"
  project               = var.project_id
  region                = var.region
  network_endpoint_type = "SERVERLESS"

  cloud_run {
    service = google_cloud_run_v2_service.docs_portal.name
  }
}

resource "google_compute_backend_service" "backend" {
  name                  = "${var.service_name}-backend"
  project               = var.project_id
  protocol              = "HTTP"
  load_balancing_scheme = "EXTERNAL_MANAGED"
  enable_cdn            = false

  backend {
    group = google_compute_region_network_endpoint_group.serverless_neg.id
  }

  dynamic "iap" {
    for_each = var.iap_enabled && var.iap_client_id != "" ? [1] : []
    content {
      oauth2_client_id     = var.iap_client_id
      oauth2_client_secret = var.iap_client_secret
    }
  }
}

resource "google_iap_web_backend_service_iam_member" "licensed_domains" {
  for_each = local.normalized_licensed_domains

  project             = var.project_id
  web_backend_service = google_compute_backend_service.backend.name
  role                = "roles/iap.httpsResourceAccessor"
  member              = each.value
}

resource "google_compute_global_address" "docs_portal" {
  name    = "${var.service_name}-ip"
  project = var.project_id
}

resource "google_compute_managed_ssl_certificate" "docs_portal" {
  name    = "${var.service_name}-cert"
  project = var.project_id

  managed {
    domains = [var.domain_name]
  }
}

resource "google_compute_url_map" "docs_portal" {
  name            = "${var.service_name}-url-map"
  project         = var.project_id
  default_service = google_compute_backend_service.backend.id
}

resource "google_compute_target_https_proxy" "docs_portal" {
  name             = "${var.service_name}-target-proxy"
  project          = var.project_id
  url_map          = google_compute_url_map.docs_portal.id
  ssl_certificates = [google_compute_managed_ssl_certificate.docs_portal.id]
}

resource "google_compute_global_forwarding_rule" "docs_portal_https" {
  name                  = "${var.service_name}-https-rule"
  project               = var.project_id
  target                = google_compute_target_https_proxy.docs_portal.id
  ip_address            = google_compute_global_address.docs_portal.address
  port_range            = "443"
  load_balancing_scheme = "EXTERNAL_MANAGED"
}

resource "google_compute_url_map" "https_redirect" {
  name    = "${var.service_name}-http-redirect"
  project = var.project_id

  default_url_redirect {
    https_redirect = true
    strip_query    = false
  }
}

resource "google_compute_target_http_proxy" "https_redirect" {
  name    = "${var.service_name}-http-proxy"
  project = var.project_id
  url_map = google_compute_url_map.https_redirect.id
}

resource "google_compute_global_forwarding_rule" "docs_portal_http" {
  name                  = "${var.service_name}-http-rule"
  project               = var.project_id
  target                = google_compute_target_http_proxy.https_redirect.id
  ip_address            = google_compute_global_address.docs_portal.address
  port_range            = "80"
  load_balancing_scheme = "EXTERNAL_MANAGED"
}
