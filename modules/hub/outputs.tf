output "service_url" {
  description = "The URI of the Hub Cloud Run service"
  value       = google_cloud_run_v2_service.hub.uri
}

output "service_name" {
  description = "The name of the Hub Cloud Run service"
  value       = google_cloud_run_v2_service.hub.name
}

output "deployment_project_id" {
  description = "The deployment GCP project ID"
  value       = var.project_id
}

output "iap_client_id" {
  description = "The OAuth 2.0 Client ID for IAP"
  value       = var.iap_client_id
}

output "contract_changed_topic_id" {
  description = "Pub/Sub topic ID for contract changed events"
  value       = google_pubsub_topic.contract_changed.id
}

output "contract_deleted_topic_id" {
  description = "Pub/Sub topic ID for contract deleted events"
  value       = google_pubsub_topic.contract_deleted.id
}

output "shovel_service_account_email" {
  description = "The service account email of the Metadata Shovel service"
  value       = google_service_account.shovel_runtime.email
}

output "audit_dataset_id" {
  description = "The BigQuery dataset ID for outbox audit events"
  value       = var.enable_audit_sink ? google_bigquery_dataset.supercargo_audit[0].dataset_id : null
}

output "audit_table_id" {
  description = "The BigQuery raw table ID for outbox audit events"
  value       = var.enable_audit_sink ? google_bigquery_table.outbox_events[0].table_id : null
}

output "audit_view_id" {
  description = "The BigQuery view ID for canonical outbox audit events"
  value       = var.enable_audit_sink ? google_bigquery_table.outbox_events_view[0].table_id : null
}

output "audit_subscription_id" {
  description = "The Pub/Sub subscription ID for the BigQuery audit sink"
  value       = var.enable_audit_sink ? google_pubsub_subscription.outbox_audit_bq[0].id : null
}
