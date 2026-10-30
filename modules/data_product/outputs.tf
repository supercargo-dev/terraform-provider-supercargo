output "product_urn" {
  description = "The URN of the registered Data Product"
  value       = data.supercargo_data_product.this.urn
}

output "gateway_service_name" {
  description = "The name of the Gateway Cloud Run service"
  value       = module.gateway.service_name
}

output "gateway_url" {
  description = "The URL of the deployed Cloud Run Gateway"
  value       = module.gateway.service_url
}

output "raw_topic_id" {
  description = "The ID of the Raw topic"
  value       = module.gateway.raw_topic_id
}

output "clean_topic_id" {
  description = "The ID of the Clean topic"
  value       = module.gateway.clean_topic_id
}

output "dlq_topic_id" {
  description = "The ID of the Dead Letter Queue topic"
  value       = module.gateway.dlq_topic_id
}

output "push_invoker_service_account_email" {
  description = "The email of the Pub/Sub Push Invoker Service Account"
  value       = module.gateway.push_invoker_service_account_email
}

output "raw_subscription_id" {
  description = "The ID of the Raw Pub/Sub Push Subscription"
  value       = module.gateway.raw_subscription_id
}

output "dlq_subscription_id" {
  description = "The ID of the Dead Letter Queue Subscription"
  value       = module.gateway.dlq_subscription_id
}

output "bigquery_table_ids" {
  description = "A map of contract names to BigQuery table IDs"
  value       = module.gateway.bigquery_table_ids
}

output "service_account_email" {
  description = "The service account email of the Gateway"
  value       = module.gateway.service_account_email
}

output "service_account_id" {
  description = "The account ID of the Gateway Service Account"
  value       = module.gateway.service_account_id
}

output "contracts" {
  description = "Map of auto-discovered or explicitly provided contracts"
  value       = data.supercargo_data_product.this.contracts
}

output "managed_input_topics" {
  description = "Map of managed input port names to their created Pub/Sub topic IDs"
  value       = { for k, v in google_pubsub_topic.managed_input : k => v.id }
}

output "input_subscription_ids" {
  description = "Map of input port names to their Pub/Sub push subscription IDs"
  value       = { for k, v in google_pubsub_subscription.input_push : k => v.id }
}

output "dlq_alert_policy_undelivered_id" {
  description = "The ID of the Cloud Monitoring alert policy for DLQ undelivered messages"
  value       = module.gateway.dlq_alert_policy_undelivered_id
}

output "dlq_alert_policy_age_id" {
  description = "The ID of the Cloud Monitoring alert policy for DLQ oldest unacked message age"
  value       = module.gateway.dlq_alert_policy_age_id
}

output "dlt_cloud_run_job_names" {
  description = "Map of DLT input port names to their Cloud Run v2 Job names"
  value       = { for k, v in google_cloud_run_v2_job.dlt_jobs : k => v.name }
}

output "dlt_cloud_scheduler_job_ids" {
  description = "Map of DLT input port names to their Cloud Scheduler job IDs"
  value       = { for k, v in google_cloud_scheduler_job.dlt_schedules : k => v.id }
}

output "dlt_backfill_commands" {
  description = "Ready-to-run gcloud commands for ad-hoc DLT historical backfills"
  value       = { for k, v in local.dlt_input_ports : k => "gcloud run jobs execute ${google_cloud_run_v2_job.dlt_jobs[k].name} --region ${var.region} --project ${var.project_id} --args=\"--since=2026-01-01T00:00:00Z\"" }
}

output "dlt_runner_service_account_email" {
  description = "The email of the DLT Runner Service Account (null if no DLT ports declared)"
  value       = length(local.dlt_input_ports) > 0 ? local.dlt_runner_sa_email : null
}

output "dlt_scheduler_service_account_email" {
  description = "The email of the DLT Cloud Scheduler Service Account (null if no DLT ports declared)"
  value       = length(local.dlt_input_ports) > 0 ? local.dlt_scheduler_sa_email : null
}



