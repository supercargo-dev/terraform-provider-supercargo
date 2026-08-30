output "service_name" {
  description = "The name of the Gateway service"
  value       = google_cloud_run_v2_service.gateway.name
}

output "service_url" {
  description = "The URL of the Gateway service"
  value       = google_cloud_run_v2_service.gateway.uri
}

output "raw_topic_id" {
  description = "The ID of the Raw topic"
  value       = google_pubsub_topic.raw.id
}

output "clean_topic_id" {
  description = "The ID of the Clean topic"
  value       = google_pubsub_topic.clean.id
}

output "dlq_topic_id" {
  description = "The ID of the Dead Letter Queue topic"
  value       = google_pubsub_topic.dlq.id
}

output "bigquery_table_ids" {
  description = "A map of contract names to BigQuery table IDs"
  value       = { for k, v in google_bigquery_table.validated_data : k => v.id }
}

output "service_account_email" {
  description = "The email of the Gateway Service Account"
  value       = google_service_account.gateway.email
}

output "push_invoker_service_account_email" {
  description = "The email of the Pub/Sub Push Invoker Service Account"
  value       = google_service_account.push_invoker.email
}

output "raw_subscription_id" {
  description = "The ID of the Raw push subscription"
  value       = google_pubsub_subscription.raw_push.id
}

output "dlq_subscription_id" {
  description = "The ID of the Dead Letter Queue subscription"
  value       = google_pubsub_subscription.dlq_sub.id
}

