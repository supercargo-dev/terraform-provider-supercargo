output "service_url" {
  description = "The URL of the Loader service"
  value       = google_cloud_run_v2_service.loader.uri
}

output "service_account_email" {
  description = "The service account email for the Loader"
  value       = google_service_account.loader_sa.email
}
