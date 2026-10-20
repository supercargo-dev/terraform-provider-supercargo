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

output "analytics_hub_listing_id" {
  description = "Resource ID of the Analytics Hub listing"
  value       = try(google_bigquery_analytics_hub_listing.telemetry_listing[0].id, null)
}

output "sharing_enabled" {
  description = "Whether global telemetry sharing is active"
  value       = var.enable_global_telemetry_share
}
