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

output "bigquery_table_ids" {
  description = "A map of contract names to BigQuery table IDs"
  value       = module.gateway.bigquery_table_ids
}
