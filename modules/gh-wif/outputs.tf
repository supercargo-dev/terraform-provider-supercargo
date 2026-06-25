output "provider_name" {
  description = "The full identifier of the Workload Identity Pool Provider"
  value       = google_iam_workload_identity_pool_provider.provider.name
}

output "service_account_email" {
  description = "The email of the Service Account for GitHub Actions"
  value       = google_service_account.github_actions.email
}
