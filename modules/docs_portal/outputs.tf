output "load_balancer_ip" {
  description = "The static global external IP address assigned to the Cloud Load Balancer"
  value       = google_compute_global_address.docs_portal.address
}

output "service_url" {
  description = "The authoritative HTTPS URL for the docs portal"
  value       = "https://${var.domain_name}"
}

output "backend_service_id" {
  description = "The ID of the GCLB backend service"
  value       = google_compute_backend_service.backend.id
}
