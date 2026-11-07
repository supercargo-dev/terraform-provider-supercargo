output "dashboard_sre_id" {
  description = "Resource ID of the Control Plane SRE Infrastructure dashboard"
  value       = try(google_monitoring_dashboard.control_plane_sre[0].id, "")
}

output "dashboard_sre_url" {
  description = "Direct GCP Console URL for the Control Plane SRE Infrastructure dashboard"
  value       = length(google_monitoring_dashboard.control_plane_sre) > 0 ? "https://console.cloud.google.com/monitoring/dashboards/custom/${element(split("/", google_monitoring_dashboard.control_plane_sre[0].id), length(split("/", google_monitoring_dashboard.control_plane_sre[0].id)) - 1)}?project=${var.project_id}" : ""
}

output "dashboard_governance_id" {
  description = "Resource ID of the Governance & Privacy Plane dashboard"
  value       = try(google_monitoring_dashboard.governance_privacy[0].id, "")
}

output "dashboard_governance_url" {
  description = "Direct GCP Console URL for the Governance & Privacy Plane dashboard"
  value       = length(google_monitoring_dashboard.governance_privacy) > 0 ? "https://console.cloud.google.com/monitoring/dashboards/custom/${element(split("/", google_monitoring_dashboard.governance_privacy[0].id), length(split("/", google_monitoring_dashboard.governance_privacy[0].id)) - 1)}?project=${var.project_id}" : ""
}

output "hub_custom_service_id" {
  description = "Custom service ID for Supercargo Hub"
  value       = try(google_monitoring_custom_service.hub[0].service_id, "")
}

output "vault_custom_service_id" {
  description = "Custom service ID for Supercargo Vault"
  value       = try(google_monitoring_custom_service.vault[0].service_id, "")
}

output "hub_availability_slo_id" {
  description = "Resource ID of the Hub availability SLO"
  value       = try(google_monitoring_slo.hub_availability[0].id, "")
}

output "hub_latency_slo_id" {
  description = "Resource ID of the Hub latency SLO"
  value       = try(google_monitoring_slo.hub_latency[0].id, "")
}

output "vault_availability_slo_id" {
  description = "Resource ID of the Vault availability SLO"
  value       = try(google_monitoring_slo.vault_availability[0].id, "")
}

output "vault_latency_slo_id" {
  description = "Resource ID of the Vault latency SLO"
  value       = try(google_monitoring_slo.vault_latency[0].id, "")
}

output "all_alert_notification_channels" {
  description = "Aggregated list of all alert notification channels including PagerDuty (suppressed on TIER_3)"
  value       = local.all_alert_notification_channels
}

output "tier2_alert_notification_channels" {
  description = "Aggregated list of non-paging alert notification channels (excluding PagerDuty)"
  value       = local.tier2_alert_notification_channels
}

output "unacked_message_age_threshold_seconds" {
  description = "The active unacknowledged message age threshold in seconds derived from SLA tier"
  value       = local.unacked_age_seconds
}
