resource "google_bigquery_dataset" "global_telemetry" {
  count                      = var.enable_global_telemetry_share ? 1 : 0
  project                    = var.project_id
  dataset_id                 = var.dataset_id
  location                   = var.location
  description                = "Supercargo global federated platform telemetry shared dataset"
  delete_contents_on_destroy = true

  labels = {
    governance = "supercargo"
    component  = "global_telemetry_share"
  }
}

resource "google_bigquery_dataset_access" "supercargo_reader_access" {
  count      = var.enable_global_telemetry_share ? 1 : 0
  project    = var.project_id
  dataset_id = var.dataset_id
  role       = "roles/bigquery.dataViewer"
  iam_member = var.supercargo_reader_principal

  depends_on = [google_bigquery_dataset.global_telemetry]
}

resource "google_bigquery_dataset_access" "authorized_view" {
  count      = var.enable_global_telemetry_share ? 1 : 0
  project    = var.project_id
  dataset_id = var.internal_dataset_id

  view {
    project_id = var.project_id
    dataset_id = var.dataset_id
    table_id   = "global_telemetry_digest"
  }

  depends_on = [google_bigquery_dataset.global_telemetry]
}

resource "google_bigquery_dataset_access" "authorized_view_github" {
  count      = var.enable_global_telemetry_share ? 1 : 0
  project    = var.project_id
  dataset_id = var.github_dataset_id

  view {
    project_id = var.project_id
    dataset_id = var.dataset_id
    table_id   = "global_telemetry_digest"
  }

  depends_on = [google_bigquery_dataset.global_telemetry]
}

resource "google_bigquery_dataset_access" "authorized_view_sdlc" {
  count      = var.enable_global_telemetry_share && var.enable_global_sdlc_share ? 1 : 0
  project    = var.project_id
  dataset_id = var.internal_dataset_id

  view {
    project_id = var.project_id
    dataset_id = var.dataset_id
    table_id   = "global_sdlc_efficiency_digest"
  }

  depends_on = [google_bigquery_dataset.global_telemetry]
}

resource "google_bigquery_dataset_access" "authorized_view_github_sdlc" {
  count      = var.enable_global_telemetry_share && var.enable_global_sdlc_share ? 1 : 0
  project    = var.project_id
  dataset_id = var.github_dataset_id

  view {
    project_id = var.project_id
    dataset_id = var.dataset_id
    table_id   = "global_sdlc_efficiency_digest"
  }

  depends_on = [google_bigquery_dataset.global_telemetry]
}

resource "google_bigquery_analytics_hub_listing" "telemetry_listing" {
  count            = var.enable_global_telemetry_share && var.enable_analytics_hub_listing ? 1 : 0
  project          = var.project_id
  location         = var.location
  data_exchange_id = var.data_exchange_id
  listing_id       = "telemetry_digest_${replace(var.project_id, "-", "_")}"
  display_name     = "Supercargo Platform Telemetry Digest"
  description      = "Pseudonymized operational telemetry shared under global-telemetry-digest contract"

  bigquery_dataset {
    dataset = "projects/${var.project_id}/datasets/${var.dataset_id}"
  }

  depends_on = [google_bigquery_dataset.global_telemetry]
}

resource "google_bigquery_analytics_hub_listing" "sdlc_efficiency_listing" {
  count            = var.enable_global_telemetry_share && var.enable_global_sdlc_share && var.enable_analytics_hub_listing ? 1 : 0
  project          = var.project_id
  location         = var.location
  data_exchange_id = var.data_exchange_id
  listing_id       = "sdlc_efficiency_digest_${replace(var.project_id, "-", "_")}"
  display_name     = "Supercargo SDLC Efficiency Digest"
  description      = "Pseudonymized operational SDLC efficiency metrics shared under global-sdlc-efficiency-digest contract"

  bigquery_dataset {
    dataset = "projects/${var.project_id}/datasets/${var.dataset_id}"
  }

  depends_on = [google_bigquery_dataset.global_telemetry]
}

resource "google_bigquery_analytics_hub_listing_subscription" "peer_benchmarks" {
  count            = var.enable_peer_benchmark_subscription && var.peer_benchmark_listing_id != "" ? 1 : 0
  project          = var.project_id
  location         = var.location
  data_exchange_id = var.peer_benchmark_data_exchange_id != "" ? var.peer_benchmark_data_exchange_id : var.data_exchange_id
  listing_id       = var.peer_benchmark_listing_id

  destination_dataset {
    location = var.location

    dataset_reference {
      project_id = var.project_id
      dataset_id = var.peer_benchmark_dataset_id
    }
  }

  lifecycle {
    precondition {
      condition     = var.peer_benchmark_data_exchange_id != "" || var.data_exchange_id != ""
      error_message = "When enable_peer_benchmark_subscription is true, either peer_benchmark_data_exchange_id or data_exchange_id must be non-empty."
    }
  }
}

