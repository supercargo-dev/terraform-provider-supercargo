output "service_url" {
  description = "The authoritative HTTPS URL for the docs portal"
  value       = var.domain_name != "" ? "https://${var.domain_name}" : google_cloud_run_v2_service.docs_portal.uri
}

output "cloud_run_uri" {
  description = "The default *.run.app URI generated for the Cloud Run service"
  value       = google_cloud_run_v2_service.docs_portal.uri
}

output "domain_mapping_dns_records" {
  description = "DNS resource records (A, AAAA, CNAME) required by DNS registrar for custom domain mapping"
  value       = var.domain_name != "" && length(google_cloud_run_domain_mapping.docs_portal) > 0 ? google_cloud_run_domain_mapping.docs_portal[0].status[0].resource_records : []
}
