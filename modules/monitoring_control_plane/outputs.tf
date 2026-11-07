output "dashboard_sre_id" {
  description = "Resource ID of the Control Plane SRE Infrastructure dashboard"
  value       = ""
}

output "dashboard_sre_url" {
  description = "Direct GCP Console URL for the Control Plane SRE Infrastructure dashboard"
  value       = ""
}

output "dashboard_governance_id" {
  description = "Resource ID of the Governance & Privacy Plane dashboard"
  value       = ""
}

output "dashboard_governance_url" {
  description = "Direct GCP Console URL for the Governance & Privacy Plane dashboard"
  value       = ""
}

output "hub_custom_service_id" {
  description = "Custom service ID for Supercargo Hub"
  value       = ""
}

output "vault_custom_service_id" {
  description = "Custom service ID for Supercargo Vault"
  value       = ""
}

output "hub_availability_slo_id" {
  description = "Resource ID of the Hub availability SLO"
  value       = ""
}

output "hub_latency_slo_id" {
  description = "Resource ID of the Hub latency SLO"
  value       = ""
}

output "vault_availability_slo_id" {
  description = "Resource ID of the Vault availability SLO"
  value       = ""
}

output "vault_latency_slo_id" {
  description = "Resource ID of the Vault latency SLO"
  value       = ""
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
