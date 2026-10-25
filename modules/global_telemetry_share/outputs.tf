output "dataset_id" {
  description = "The ID of the shared BigQuery dataset"
  value       = try(google_bigquery_dataset.global_telemetry[0].dataset_id, null)
}

output "dataset_access_id" {
  description = "Resource ID of the reader dataset access grant"
  value       = try(google_bigquery_dataset_access.supercargo_reader_access[0].id, null)
}

output "authorized_view_access_id" {
  description = "Resource ID of the Authorized View grant on the internal dataset"
  value       = try(google_bigquery_dataset_access.authorized_view[0].id, null)
}

output "authorized_view_github_access_id" {
  description = "Resource ID of the Authorized View grant on the GitHub dataset"
  value       = try(google_bigquery_dataset_access.authorized_view_github[0].id, null)
}

output "authorized_view_sdlc_access_id" {
  description = "Resource ID of the Authorized View grant for SDLC efficiency metrics on the internal dataset"
  value       = try(google_bigquery_dataset_access.authorized_view_sdlc[0].id, null)
}

output "authorized_view_github_sdlc_access_id" {
  description = "Resource ID of the Authorized View grant for SDLC efficiency metrics on the GitHub dataset"
  value       = try(google_bigquery_dataset_access.authorized_view_github_sdlc[0].id, null)
}

output "analytics_hub_listing_id" {
  description = "Resource ID of the Analytics Hub listing"
  value       = try(google_bigquery_analytics_hub_listing.telemetry_listing[0].id, null)
}

output "sdlc_analytics_hub_listing_id" {
  description = "Resource ID of the Analytics Hub listing for SDLC efficiency metrics"
  value       = try(google_bigquery_analytics_hub_listing.sdlc_efficiency_listing[0].id, null)
}

output "peer_benchmark_subscription_id" {
  description = "Resource ID of the reciprocal peer benchmark Analytics Hub subscription"
  value       = try(google_bigquery_analytics_hub_subscription.peer_benchmarks[0].id, null)
}

output "sharing_enabled" {
  description = "Whether global telemetry sharing is active"
  value       = var.enable_global_telemetry_share
}

output "sdlc_sharing_enabled" {
  description = "Whether global SDLC efficiency sharing is active"
  value       = var.enable_global_sdlc_share
}

output "peer_benchmarks_enabled" {
  description = "Whether peer benchmark subscription is active"
  value       = var.enable_peer_benchmark_subscription
}

