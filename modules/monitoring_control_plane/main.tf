# --- Slug & Display Name Hygiene (Whitespace Elimination) ---
locals {
  name_suffix_id      = var.name_suffix != "" ? "-${var.name_suffix}" : ""
  name_suffix_display = var.name_suffix != "" ? " - ${var.name_suffix}" : ""
}

# --- Cloud Monitoring Notification Channels ---

resource "google_monitoring_notification_channel" "slack" {
  count        = var.alert_slack_channel != "" ? 1 : 0
  project      = var.project_id
  display_name = "Slack Alert Channel (Control Plane)${local.name_suffix_display}"
  type         = "slack"
  labels = {
    "channel_name" = var.alert_slack_channel
  }
}

resource "google_monitoring_notification_channel" "email" {
  count        = var.alert_email_address != "" ? 1 : 0
  project      = var.project_id
  display_name = "Email Alert Channel (Control Plane)${local.name_suffix_display}"
  type         = "email"
  labels = {
    "email_address" = var.alert_email_address
  }
}

resource "google_monitoring_notification_channel" "pagerduty" {
  count        = var.alert_pagerduty_service_key != "" ? 1 : 0
  project      = var.project_id
  display_name = "PagerDuty Alert Channel (Control Plane)${local.name_suffix_display}"
  type         = "pagerduty"
  sensitive_labels {
    service_key = var.alert_pagerduty_service_key
  }
}

locals {
  sla_thresholds = {
    TIER_1 = {
      unacked_age_seconds = 120
      cpu_threshold       = 0.80
      memory_threshold    = 0.80
    }
    TIER_2 = {
      unacked_age_seconds = 300
      cpu_threshold       = 0.85
      memory_threshold    = 0.85
    }
    TIER_3 = {
      unacked_age_seconds = 600
      cpu_threshold       = 0.90
      memory_threshold    = 0.90
    }
  }

  active_sla          = local.sla_thresholds[var.sla_tier]
  unacked_age_seconds = coalesce(var.unacked_message_age_threshold_seconds, local.active_sla.unacked_age_seconds)

  # Non-paging Tier 2 notification channels
  tier2_alert_notification_channels = distinct(compact(concat(
    google_monitoring_notification_channel.slack[*].name,
    google_monitoring_notification_channel.email[*].name,
    var.alert_notification_channels,
    var.additional_sre_notification_channels
  )))

  # Tier 1 paging notification channels with TIER_3 suppression
  all_alert_notification_channels = distinct(compact(concat(
    local.tier2_alert_notification_channels,
    var.sla_tier != "TIER_3" ? google_monitoring_notification_channel.pagerduty[*].name : []
  )))

  # Monitored Control Plane Cloud Run Services (with empty pruning)
  raw_control_plane_services = {
    hub = {
      name          = var.hub_service_name
      max_instances = var.hub_max_instances
      display       = "Hub"
      tier          = "Tier 1"
    }
    vault = {
      name          = var.vault_service_name
      max_instances = var.vault_max_instances
      display       = "Vault"
      tier          = "Tier 1"
    }
    shovel = {
      name          = var.shovel_service_name
      max_instances = var.shovel_max_instances
      display       = "Metadata Shovel"
      tier          = "Tier 1"
    }
    mcp = {
      name          = var.mcp_service_name
      max_instances = var.mcp_max_instances
      display       = "Supercargo MCP"
      tier          = "Tier 2"
    }
    console = {
      name          = var.console_service_name
      max_instances = var.console_max_instances
      display       = "Console"
      tier          = "Tier 2"
    }
    analytics_dashboard = {
      name          = var.analytics_dashboard_service_name
      max_instances = var.analytics_dashboard_max_instances
      display       = "Analytics Dashboard"
      tier          = "Tier 2"
    }
    github_runner_webhook = {
      name          = var.github_runner_webhook_service_name
      max_instances = var.github_runner_webhook_max_instances
      display       = "GitHub Runner Webhook"
      tier          = "Tier 2"
    }
  }

  control_plane_services = {
    for k, v in local.raw_control_plane_services : k => v if v.name != null && v.name != ""
  }
}

# 1. Cloud Run HTTP 5xx Error Rate Alert
resource "google_monitoring_alert_policy" "service_5xx_rate" {
  for_each     = var.enable_control_plane_alerts ? local.control_plane_services : {}
  project      = var.project_id
  display_name = "[Tier 1] [INFRA] Cloud Run 5xx Errors (${each.value.display})${local.name_suffix_display}"
  combiner     = "OR"

  conditions {
    display_name = "Cloud Run 5xx Response Rate > 0"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${each.value.name}\" AND metric.type = \"run.googleapis.com/request_count\" AND metric.label.response_code_class = \"5xx\""
      duration        = "0s"
      comparison      = "COMPARISON_GT"
      threshold_value = 0
      trigger {
        count = 1
      }
      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_RATE"
      }
    }
  }

  documentation {
    content   = <<-EOT
      ## [Tier 1] Cloud Run 5xx Errors Detected
      - **Service**: `${each.value.name}`

      ### Triage CLI Command
      ```bash
      gcloud beta run services logs read ${each.value.name} --region=${var.region} --limit=50
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = each.value.tier == "Tier 1" ? local.all_alert_notification_channels : local.tier2_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# 2. Cloud Run HTTP 401/403 Authentication/Authorization Rejection Alert
resource "google_monitoring_alert_policy" "service_auth_failures" {
  for_each     = var.enable_control_plane_alerts ? local.control_plane_services : {}
  project      = var.project_id
  display_name = "[Tier 1] [INFRA] Cloud Run Invoker Auth 401/403 (${each.value.display})${local.name_suffix_display}"
  combiner     = "OR"

  conditions {
    display_name = "Cloud Run 401/403 Auth Rejection Rate > 0"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${each.value.name}\" AND metric.type = \"run.googleapis.com/request_count\" AND metric.label.response_code = one_of(\"401\", \"403\")"
      duration        = "0s"
      comparison      = "COMPARISON_GT"
      threshold_value = 0
      trigger {
        count = 1
      }
      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_RATE"
      }
    }
  }

  documentation {
    content   = <<-EOT
      ## [Tier 1] Cloud Run Authentication/Authorization Rejection
      - **Service**: `${each.value.name}`

      ### Triage CLI Command
      ```bash
      gcloud run services get-iam-policy ${each.value.name} --region=${var.region} --flatten=\"bindings[].members\" --filter=\"bindings.role:roles/run.invoker\"
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = each.value.tier == "Tier 1" ? local.all_alert_notification_channels : local.tier2_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# 3. Cloud Run Container Memory Saturation (>85%) Alert
resource "google_monitoring_alert_policy" "service_memory_utilization" {
  for_each     = var.enable_control_plane_alerts ? local.control_plane_services : {}
  project      = var.project_id
  display_name = "[Tier 2] [INFRA] Cloud Run Memory Saturation >85% (${each.value.display})${local.name_suffix_display}"
  combiner     = "OR"

  conditions {
    display_name = "Memory Utilization Sustained > 85%"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${each.value.name}\" AND metric.type = \"run.googleapis.com/container/memory/utilizations\""
      duration        = "300s"
      comparison      = "COMPARISON_GT"
      threshold_value = local.active_sla.memory_threshold
      trigger {
        count = 1
      }
      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_PERCENTILE_99"
      }
    }
  }

  documentation {
    content   = <<-EOT
      ## [Tier 2] Cloud Run Container Memory Saturation
      - **Service**: `${each.value.name}`
      - **Threshold**: `${local.active_sla.memory_threshold * 100}%`

      ### Triage CLI Command
      ```bash
      gcloud run services describe ${each.value.name} --region=${var.region} --format=\"value(spec.template.spec.containers[0].resources.limits.memory)\"
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.tier2_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# 4. Cloud Run Container CPU Saturation (>85%) Alert
resource "google_monitoring_alert_policy" "service_cpu_utilization" {
  for_each     = var.enable_control_plane_alerts ? local.control_plane_services : {}
  project      = var.project_id
  display_name = "[Tier 2] [INFRA] Cloud Run CPU Saturation >85% (${each.value.display})${local.name_suffix_display}"
  combiner     = "OR"

  conditions {
    display_name = "CPU Utilization Sustained > 85%"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${each.value.name}\" AND metric.type = \"run.googleapis.com/container/cpu/utilizations\""
      duration        = "300s"
      comparison      = "COMPARISON_GT"
      threshold_value = local.active_sla.cpu_threshold
      trigger {
        count = 1
      }
      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_PERCENTILE_99"
      }
    }
  }

  documentation {
    content   = <<-EOT
      ## [Tier 2] Cloud Run Container CPU Saturation
      - **Service**: `${each.value.name}`
      - **Threshold**: `${local.active_sla.cpu_threshold * 100}%`

      ### Triage CLI Command
      ```bash
      gcloud run services describe ${each.value.name} --region=${var.region} --format=\"value(spec.template.spec.containers[0].resources.limits.cpu)\"
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.tier2_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# 5. Cloud Run Active Container Instance Saturation Alert
resource "google_monitoring_alert_policy" "service_instance_saturation" {
  for_each     = var.enable_control_plane_alerts ? local.control_plane_services : {}
  project      = var.project_id
  display_name = "[Tier 2] [INFRA] Cloud Run Instance Saturation (${each.value.display})${local.name_suffix_display}"
  combiner     = "OR"

  conditions {
    display_name = "Active Instances Reached Max Limit"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${each.value.name}\" AND metric.type = \"run.googleapis.com/container/instance_count\" AND metric.label.state = \"active\""
      duration        = "180s"
      comparison      = "COMPARISON_GTE"
      threshold_value = each.value.max_instances
      trigger {
        count = 1
      }
      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_MAX"
      }
    }
  }

  documentation {
    content   = <<-EOT
      ## [Tier 2] Cloud Run Active Instance Ceiling Reached
      - **Service**: `${each.value.name}`
      - **Max Instances**: `${each.value.max_instances}`

      ### Triage CLI Command
      ```bash
      gcloud run services describe ${each.value.name} --region=${var.region} --format=\"value(spec.template.metadata.annotations['autoscaling.knative.dev/maxScale'])\"
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.tier2_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}
