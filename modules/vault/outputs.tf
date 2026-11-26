output "vault_service_account_email" {
  value = google_service_account.vault_sa.email
}

output "service_account_email" {
  value = google_service_account.vault_sa.email
}

output "master_key_id" {
  value = google_kms_crypto_key.master_key.id
}

output "service_url" {
  value = google_cloud_run_v2_service.vault.uri
}

output "service_name" {
  value = google_cloud_run_v2_service.vault.name
}

output "search_hash_pam_entitlement_id" {
  description = "Provisioned GCP PAM entitlement ID for Vault SearchHash JIT access (null when disabled)"
  value       = try(google_privileged_access_manager_entitlement.vault_search_hash_jit["search_hash"].entitlement_id, null)
}

output "search_hash_pam_gcloud_command" {
  description = "Ready-to-run gcloud beta pam grants create command for Vault SearchHash JIT access (null when disabled)"
  value       = try("gcloud beta pam grants create --entitlement=${google_privileged_access_manager_entitlement.vault_search_hash_jit["search_hash"].entitlement_id} --requested-duration=1800s --justification=\"TICKET-123: reason\" --location=${var.search_hash_pam_location} --project=${var.project_id}", null)
}

output "search_hash_pam_console_url" {
  description = "GCP Console PAM entitlement deep-link for Vault SearchHash JIT access (null when disabled)"
  value       = try("https://console.cloud.google.com/iam-admin/pam/entitlements/detail/${var.search_hash_pam_location}/${google_privileged_access_manager_entitlement.vault_search_hash_jit["search_hash"].entitlement_id}?project=${var.project_id}", null)
}
