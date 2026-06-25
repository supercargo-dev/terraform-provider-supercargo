output "vault_service_account_email" {
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

output "iap_client_id" {
  value = var.iap_client_id
}
