locals {
  hub_custom_service_id   = "supercargo-hub-${var.environment}${local.name_suffix_id}"
  vault_custom_service_id = "supercargo-vault-${var.environment}${local.name_suffix_id}"

  hub_slo_prefix   = "projects/${var.project_id}/services/${local.hub_custom_service_id}/serviceLevelObjectives"
  vault_slo_prefix = "projects/${var.project_id}/services/${local.vault_custom_service_id}/serviceLevelObjectives"

  raw_slo_alerts = {
    hub_avail_fast = {
      enabled      = var.enable_slos && var.hub_service_name != "" && var.hub_service_name != null
      display_name = "[Tier 1] [SLO] Fast Burn: Hub Availability (1h, 14.4x)"
      slo_id       = "${local.hub_slo_prefix}/hub-availability"
      window       = "3600s"
      threshold    = 14.4
      channels     = local.all_alert_notification_channels
      service_name = var.hub_service_name
    }
    hub_avail_slow = {
      enabled      = var.enable_slos && var.hub_service_name != "" && var.hub_service_name != null
      display_name = "[Tier 2] [SLO] Slow Burn: Hub Availability (24h, 1.0x)"
      slo_id       = "${local.hub_slo_prefix}/hub-availability"
      window       = "86400s"
      threshold    = 1.0
      channels     = local.tier2_alert_notification_channels
      service_name = var.hub_service_name
    }
    hub_latency_fast = {
      enabled      = var.enable_slos && var.hub_service_name != "" && var.hub_service_name != null
      display_name = "[Tier 1] [SLO] Fast Burn: Hub Latency P99 (1h, 14.4x)"
      slo_id       = "${local.hub_slo_prefix}/hub-latency"
      window       = "3600s"
      threshold    = 14.4
      channels     = local.all_alert_notification_channels
      service_name = var.hub_service_name
    }
    hub_latency_slow = {
      enabled      = var.enable_slos && var.hub_service_name != "" && var.hub_service_name != null
      display_name = "[Tier 2] [SLO] Slow Burn: Hub Latency P99 (24h, 1.0x)"
      slo_id       = "${local.hub_slo_prefix}/hub-latency"
      window       = "86400s"
      threshold    = 1.0
      channels     = local.tier2_alert_notification_channels
      service_name = var.hub_service_name
    }
    vault_avail_fast = {
      enabled      = var.enable_slos && var.vault_service_name != "" && var.vault_service_name != null
      display_name = "[Tier 1] [SLO] Fast Burn: Vault Availability (1h, 14.4x)"
      slo_id       = "${local.vault_slo_prefix}/vault-availability"
      window       = "3600s"
      threshold    = 14.4
      channels     = local.all_alert_notification_channels
      service_name = var.vault_service_name
    }
    vault_avail_slow = {
      enabled      = var.enable_slos && var.vault_service_name != "" && var.vault_service_name != null
      display_name = "[Tier 2] [SLO] Slow Burn: Vault Availability (24h, 1.0x)"
      slo_id       = "${local.vault_slo_prefix}/vault-availability"
      window       = "86400s"
      threshold    = 1.0
      channels     = local.tier2_alert_notification_channels
      service_name = var.vault_service_name
    }
    vault_latency_fast = {
      enabled      = var.enable_slos && var.vault_service_name != "" && var.vault_service_name != null
      display_name = "[Tier 1] [SLO] Fast Burn: Vault Latency P99 (1h, 14.4x)"
      slo_id       = "${local.vault_slo_prefix}/vault-latency"
      window       = "3600s"
      threshold    = 14.4
      channels     = local.all_alert_notification_channels
      service_name = var.vault_service_name
    }
    vault_latency_slow = {
      enabled      = var.enable_slos && var.vault_service_name != "" && var.vault_service_name != null
      display_name = "[Tier 2] [SLO] Slow Burn: Vault Latency P99 (24h, 1.0x)"
      slo_id       = "${local.vault_slo_prefix}/vault-latency"
      window       = "86400s"
      threshold    = 1.0
      channels     = local.tier2_alert_notification_channels
      service_name = var.vault_service_name
    }
  }

  slo_alerts = {
    for k, v in local.raw_slo_alerts : k => v if v.enabled
  }
}

# --- Custom Service Definitions ---

resource "google_monitoring_custom_service" "hub" {
  count        = var.enable_slos && var.hub_service_name != "" && var.hub_service_name != null ? 1 : 0
  service_id   = local.hub_custom_service_id
  display_name = "Supercargo Hub (${var.environment})${local.name_suffix_display}"
  project      = var.project_id
}

resource "google_monitoring_custom_service" "vault" {
  count        = var.enable_slos && var.vault_service_name != "" && var.vault_service_name != null ? 1 : 0
  service_id   = local.vault_custom_service_id
  display_name = "Supercargo Vault (${var.environment})${local.name_suffix_display}"
  project      = var.project_id
}

# --- Hub Service Level Objectives ---

resource "google_monitoring_slo" "hub_availability" {
  count               = var.enable_slos && var.hub_service_name != "" && var.hub_service_name != null ? 1 : 0
  service             = local.hub_custom_service_id
  slo_id              = "hub-availability"
  display_name        = "Hub 99.9% Availability"
  project             = var.project_id
  goal                = 0.999
  rolling_period_days = 30
  depends_on          = [google_monitoring_custom_service.hub]

  request_based_sli {
    good_total_ratio {
      good_service_filter  = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${var.hub_service_name}\" AND metric.type = \"run.googleapis.com/request_count\" AND metric.label.response_code_class != \"5xx\""
      total_service_filter = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${var.hub_service_name}\" AND metric.type = \"run.googleapis.com/request_count\""
    }
  }
}

resource "google_monitoring_slo" "hub_latency" {
  count               = var.enable_slos && var.hub_service_name != "" && var.hub_service_name != null ? 1 : 0
  service             = local.hub_custom_service_id
  slo_id              = "hub-latency"
  display_name        = "Hub Latency P99 < 100ms"
  project             = var.project_id
  goal                = 0.99
  rolling_period_days = 30
  depends_on          = [google_monitoring_custom_service.hub]

  request_based_sli {
    distribution_cut {
      distribution_filter = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${var.hub_service_name}\" AND metric.type = \"run.googleapis.com/request_latencies\""
      range {
        max = 100
      }
    }
  }
}

# --- Vault Service Level Objectives ---

resource "google_monitoring_slo" "vault_availability" {
  count               = var.enable_slos && var.vault_service_name != "" && var.vault_service_name != null ? 1 : 0
  service             = local.vault_custom_service_id
  slo_id              = "vault-availability"
  display_name        = "Vault 99.95% Availability"
  project             = var.project_id
  goal                = 0.9995
  rolling_period_days = 30
  depends_on          = [google_monitoring_custom_service.vault]

  request_based_sli {
    good_total_ratio {
      good_service_filter  = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${var.vault_service_name}\" AND metric.type = \"run.googleapis.com/request_count\" AND metric.label.response_code_class != \"5xx\""
      total_service_filter = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${var.vault_service_name}\" AND metric.type = \"run.googleapis.com/request_count\""
    }
  }
}

resource "google_monitoring_slo" "vault_latency" {
  count               = var.enable_slos && var.vault_service_name != "" && var.vault_service_name != null ? 1 : 0
  service             = local.vault_custom_service_id
  slo_id              = "vault-latency"
  display_name        = "Vault Latency P99 < 50ms"
  project             = var.project_id
  goal                = 0.99
  rolling_period_days = 30
  depends_on          = [google_monitoring_custom_service.vault]

  request_based_sli {
    distribution_cut {
      distribution_filter = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${var.vault_service_name}\" AND metric.type = \"run.googleapis.com/request_latencies\""
      range {
        max = 50
      }
    }
  }
}

# --- Multi-Window Burn-Rate Alert Policies ---

resource "google_monitoring_alert_policy" "slo_burn_rate" {
  for_each     = local.slo_alerts
  project      = var.project_id
  display_name = "${each.value.display_name}${local.name_suffix_display}"
  combiner     = "OR"

  conditions {
    display_name = "SLO Burn Rate Exceeded"
    condition_threshold {
      filter          = "select_slo_burn_rate(\"${each.value.slo_id}\", \"${each.value.window}\")"
      duration        = "0s"
      comparison      = "COMPARISON_GT"
      threshold_value = each.value.threshold
      trigger {
        count = 1
      }
    }
  }

  documentation {
    content   = <<-EOT
      ## SLO Error Budget Consumption Alert
      - **Policy**: `${each.value.display_name}`
      - **Window**: `${each.value.window}`
      - **Threshold**: `${each.value.threshold}x burn rate`

      ### Triage CLI Command
      ```bash
      gcloud beta run services logs read ${each.value.service_name} --region=${var.region} --limit=50
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = each.value.channels
  alert_strategy {
    auto_close = "604800s"
  }

  depends_on = [
    google_monitoring_slo.hub_availability,
    google_monitoring_slo.hub_latency,
    google_monitoring_slo.vault_availability,
    google_monitoring_slo.vault_latency
  ]
}
