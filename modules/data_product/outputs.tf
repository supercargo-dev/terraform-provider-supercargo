output "product_urn" {
  description = "The URN of the registered Data Product"
  value       = supercargo_data_product.this.urn
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

output "contracts" {
  description = "Map of auto-discovered or explicitly provided contracts"
  value       = supercargo_data_product.this.contracts
}

output "managed_input_topics" {
  description = "Map of managed input port names to their created Pub/Sub topic IDs"
  value       = { for k, v in google_pubsub_topic.managed_input : k => v.id }
}

output "input_subscription_ids" {
  description = "Map of input port names to their Pub/Sub push subscription IDs"
  value       = { for k, v in google_pubsub_subscription.input_push : k => v.id }
}


