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

output "mcp_service_url" {
  description = "The URI of the Supercargo MCP companion Cloud Run service"
  value       = var.mcp_enabled ? google_cloud_run_v2_service.mcp[0].uri : null
}

output "mcp_service_name" {
  description = "The name of the Supercargo MCP companion Cloud Run service"
  value       = var.mcp_enabled ? google_cloud_run_v2_service.mcp[0].name : null
}

output "mcp_service_account_email" {
  description = "The service account email of the Supercargo MCP companion service"
  value       = var.mcp_enabled ? google_service_account.mcp_runtime[0].email : null
}

output "health_events_topic_name" {
  description = "The Pub/Sub topic name for asset health transition events"
  value       = var.enable_health_mesh ? google_pubsub_topic.health_events[0].name : null
}

output "health_events_topic_id" {
  description = "The Pub/Sub topic ID for asset health transition events"
  value       = var.enable_health_mesh ? google_pubsub_topic.health_events[0].id : null
}

output "catalog_dataset_id" {
  description = "The BigQuery dataset ID for asset catalog and health history"
  value       = var.enable_health_mesh ? google_bigquery_dataset.supercargo_catalog[0].dataset_id : null
}

output "asset_health_history_table_id" {
  description = "The BigQuery table ID for asset health transition history"
  value       = var.enable_health_mesh ? google_bigquery_table.asset_health_history[0].table_id : null
}

output "asset_current_health_view_id" {
  description = "The BigQuery view ID for latest asset health analytical projection"
  value       = var.enable_health_mesh ? google_bigquery_table.asset_current_health[0].table_id : null
}

output "health_events_subscription_name" {
  description = "The Pub/Sub BigQuery subscription name for health events"
  value       = var.enable_health_mesh ? google_pubsub_subscription.health_events_bq[0].name : null
}

