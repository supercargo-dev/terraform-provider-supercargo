# Cloud Monitoring Notification Channels

locals {
  metric_suffix       = var.name_suffix != "" ? "_${replace(var.name_suffix, "-", "_")}" : ""
  name_suffix_display = var.name_suffix != "" ? " - ${var.name_suffix}" : ""
}

resource "google_monitoring_notification_channel" "slack" {
  count        = var.alert_slack_channel != "" ? 1 : 0
  project      = var.project_id
  display_name = "Slack Alert Channel (${var.product_id})${local.name_suffix_display}"
  type         = "slack"
  labels = {
    "channel_name" = var.alert_slack_channel
  }
  depends_on = [time_sleep.wait_for_gateway_apis]
}

resource "google_monitoring_notification_channel" "email" {
  count        = var.alert_email_address != "" ? 1 : 0
  project      = var.project_id
  display_name = "Email Alert Channel (${var.product_id})${local.name_suffix_display}"
  type         = "email"
  labels = {
    "email_address" = var.alert_email_address
  }
  depends_on = [time_sleep.wait_for_gateway_apis]
}

resource "google_monitoring_notification_channel" "pagerduty" {
  count        = var.alert_pagerduty_service_key != "" ? 1 : 0
  project      = var.project_id
  display_name = "PagerDuty Alert Channel (${var.product_id})${local.name_suffix_display}"
  type         = "pagerduty"
  sensitive_labels {
    service_key = var.alert_pagerduty_service_key
  }
  depends_on = [time_sleep.wait_for_gateway_apis]
}

locals {
  sla_thresholds = {
    TIER_1 = {
      unacked_age_seconds = 120
      cpu_threshold       = 0.80
      memory_threshold    = 0.80
      error_rate_budget   = 0.05
    }
    TIER_2 = {
      unacked_age_seconds = 300
      cpu_threshold       = 0.85
      memory_threshold    = 0.85
      error_rate_budget   = 0.20
    }
    TIER_3 = {
      unacked_age_seconds = 600
      cpu_threshold       = 0.90
      memory_threshold    = 0.90
      error_rate_budget   = 0.40
    }
  }

  active_sla          = local.sla_thresholds[var.sla_tier]
  unacked_age_seconds = coalesce(var.unacked_message_age_threshold_seconds, local.active_sla.unacked_age_seconds)

  # Multi-Channel Routing Hygiene (#396)
  tier2_alert_notification_channels = distinct(compact(concat(
    google_monitoring_notification_channel.slack[*].name,
    google_monitoring_notification_channel.email[*].name,
    var.alert_notification_channels,
    var.additional_sre_notification_channels
  )))

  # Suppress PagerDuty on TIER_3 to eliminate off-hours alert fatigue
  all_alert_notification_channels = distinct(compact(concat(
    local.tier2_alert_notification_channels,
    # google_monitoring_notification_channel.slack
    # google_monitoring_notification_channel.email
    # var.alert_notification_channels
    var.sla_tier != "TIER_3" ? google_monitoring_notification_channel.pagerduty[*].name : []
  )))

  # Documentation Truncation Hygiene with contracts count indicator (#397)
  contract_ids                = [for k, v in var.contracts : v.id]
  contracts_count             = length(local.contract_ids)
  raw_contracts_summary       = local.contracts_count > 0 ? join(", ", local.contract_ids) : var.product_id
  monitored_contracts_summary = length(local.raw_contracts_summary) > 250 ? "${substr(local.raw_contracts_summary, 0, 240)}... (+${local.contracts_count - 1} more contracts)" : local.raw_contracts_summary
}

# --- METRIC DESCRIPTORS ---

resource "google_monitoring_metric_descriptor" "gateway_messages_total" {
  project      = var.project_id
  description  = "Total count of processed messages"
  display_name = "Gateway Messages Total"
  type         = "custom.googleapis.com/${replace(var.product_id, "-", "_")}${local.metric_suffix}_messages_total"
  metric_kind  = "CUMULATIVE"
  value_type   = "INT64"
  unit         = "1"

  labels {
    key         = "contract_urn"
    value_type  = "STRING"
    description = "The URN of the data contract"
  }
  labels {
    key         = "product_id"
    value_type  = "STRING"
    description = "The ID of the data product"
  }
  labels {
    key         = "status"
    value_type  = "STRING"
    description = "Ingestion status (success, failed, quarantine)"
  }

  depends_on = [time_sleep.wait_for_gateway_apis]

  timeouts {
    create = "5m"
    delete = "5m"
  }
}

resource "google_monitoring_metric_descriptor" "gateway_validation_duration_ms" {
  project      = var.project_id
  description  = "Latency of message validation"
  display_name = "Gateway Validation Duration"
  type         = "custom.googleapis.com/${replace(var.product_id, "-", "_")}${local.metric_suffix}_validation_duration_ms"
  metric_kind  = "CUMULATIVE"
  value_type   = "DISTRIBUTION"
  unit         = "ms"

  labels {
    key         = "contract_urn"
    value_type  = "STRING"
    description = "The URN of the data contract"
  }
  labels {
    key         = "product_id"
    value_type  = "STRING"
    description = "The ID of the data product"
  }

  depends_on = [time_sleep.wait_for_gateway_apis]

  timeouts {
    create = "5m"
    delete = "5m"
  }
}

resource "google_monitoring_metric_descriptor" "gateway_validation_errors_total" {
  project      = var.project_id
  description  = "Total count of validation errors"
  display_name = "Gateway Validation Errors Total"
  type         = "custom.googleapis.com/${replace(var.product_id, "-", "_")}${local.metric_suffix}_validation_errors_total"
  metric_kind  = "CUMULATIVE"
  value_type   = "INT64"
  unit         = "1"

  labels {
    key         = "contract_urn"
    value_type  = "STRING"
    description = "The URN of the data contract"
  }
  labels {
    key         = "product_id"
    value_type  = "STRING"
    description = "The ID of the data product"
  }
  labels {
    key         = "error_type"
    value_type  = "STRING"
    description = "Type of validation error"
  }

  depends_on = [time_sleep.wait_for_gateway_apis]

  timeouts {
    create = "5m"
    delete = "5m"
  }
}

# --- TIER 1: MISSION CRITICAL ---

# Alert on ANY validation failure for Tier 1
resource "google_monitoring_alert_policy" "tier1_validation_failure" {
  project      = var.project_id
  display_name = "[Tier 1] Gateway Validation Failure (${var.product_id})${local.name_suffix_display}"
  combiner     = "OR"
  depends_on = [
    google_cloud_run_v2_service.gateway,
    google_monitoring_metric_descriptor.gateway_validation_errors_total
  ]
  conditions {
    display_name = "Validation Errors > 0"
    condition_threshold {
      filter          = "metric.type=\"custom.googleapis.com/${replace(var.product_id, "-", "_")}${local.metric_suffix}_validation_errors_total\" resource.type=\"global\""
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

  notification_channels = local.all_alert_notification_channels

  alert_strategy {
    auto_close = "604800s" # 7 days
  }
}

# "Silent Killer": Absence of Data for Tier 1
resource "google_monitoring_alert_policy" "tier1_absence_of_data" {
  project      = var.project_id
  display_name = "[Tier 1] Ingestion Traffic Drop - Absence of Data (${var.product_id})${local.name_suffix_display}"
  combiner     = "OR"
  depends_on = [
    google_cloud_run_v2_service.gateway,
    google_monitoring_metric_descriptor.gateway_messages_total
  ]
  conditions {
    display_name = "No messages reported"
    condition_absent {
      filter   = "metric.type=\"custom.googleapis.com/${replace(var.product_id, "-", "_")}${local.metric_suffix}_messages_total\" resource.type=\"global\""
      duration = "300s" # 5 minutes
      trigger {
        count = 1
      }
      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_RATE"
      }
    }
  }

  notification_channels = local.all_alert_notification_channels
}

# Alert on DLQ quarantined messages backlog
resource "google_monitoring_alert_policy" "dlq_undelivered_messages" {
  count        = var.enable_dlq_alerts ? 1 : 0
  project      = var.project_id
  display_name = "[Tier 1] DLQ Quarantined Messages Backlog (${var.product_id})${local.name_suffix_display}"
  combiner     = "OR"
  depends_on = [
    time_sleep.wait_for_gateway_apis,
    google_pubsub_subscription.dlq_sub
  ]

  conditions {
    display_name = "DLQ Undelivered Messages > ${var.dlq_alert_threshold}"
    condition_threshold {
      filter          = "resource.type = \"pubsub_subscription\" AND resource.labels.subscription_id = \"${google_pubsub_subscription.dlq_sub.name}\" AND metric.type = \"pubsub.googleapis.com/subscription/num_undelivered_messages\""
      duration        = "0s"
      comparison      = "COMPARISON_GT"
      threshold_value = var.dlq_alert_threshold
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
      ## Dead-Letter Queue (DLQ) Quarantined Messages Backlog

      - **Product ID**: `${var.product_id}`
      - **Contract URN(s)**: `${local.monitored_contracts_summary}`
      - **DLQ Topic ID**: `${google_pubsub_topic.dlq.id}`
      - **DLQ Subscription ID**: `${google_pubsub_subscription.dlq_sub.id}`

      ### Triage CLI Command
      ```bash
      gcloud pubsub subscriptions pull ${google_pubsub_subscription.dlq_sub.name} --limit=5 --auto-ack=false
      ```

      ### Remediation Runbook
      Consult the incident runbook: [DLQ Remediation Guide](${var.dlq_runbook_url})
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.all_alert_notification_channels

  alert_strategy {
    auto_close = "604800s" # 7 days
  }
}

# Alert on DLQ message latency exceeded
resource "google_monitoring_alert_policy" "dlq_message_age" {
  count        = var.enable_dlq_alerts ? 1 : 0
  project      = var.project_id
  display_name = "[Tier 1] DLQ Message Latency Exceeded (${var.product_id})${local.name_suffix_display}"
  combiner     = "OR"
  depends_on = [
    time_sleep.wait_for_gateway_apis,
    google_pubsub_subscription.dlq_sub
  ]

  conditions {
    display_name = "DLQ Message Age > ${var.dlq_unacked_message_age_seconds}s"
    condition_threshold {
      filter          = "resource.type = \"pubsub_subscription\" AND resource.labels.subscription_id = \"${google_pubsub_subscription.dlq_sub.name}\" AND metric.type = \"pubsub.googleapis.com/subscription/oldest_unacked_message_age\""
      duration        = "0s"
      comparison      = "COMPARISON_GT"
      threshold_value = var.dlq_unacked_message_age_seconds
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
      ## Dead-Letter Queue (DLQ) Message Latency Exceeded

      - **Product ID**: `${var.product_id}`
      - **Contract URN(s)**: `${local.monitored_contracts_summary}`
      - **DLQ Topic ID**: `${google_pubsub_topic.dlq.id}`
      - **DLQ Subscription ID**: `${google_pubsub_subscription.dlq_sub.id}`

      ### Triage CLI Command
      ```bash
      gcloud pubsub subscriptions pull ${google_pubsub_subscription.dlq_sub.name} --limit=5 --auto-ack=false
      ```

      ### Remediation Runbook
      Consult the incident runbook: [DLQ Remediation Guide](${var.dlq_runbook_url})
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.all_alert_notification_channels

  alert_strategy {
    auto_close = "604800s" # 7 days
  }
}

# --- TIER 2: IMPORTANT ---

# Alert on P99 Latency > 100ms
resource "google_monitoring_alert_policy" "tier2_latency_spike" {
  project      = var.project_id
  display_name = "[Tier 2] Gateway Latency Spike P99 > 100ms (${var.product_id})${local.name_suffix_display}"
  combiner     = "OR"
  depends_on = [
    google_cloud_run_v2_service.gateway,
    google_monitoring_metric_descriptor.gateway_validation_duration_ms
  ]
  conditions {
    display_name = "P99 Latency High"
    condition_monitoring_query_language {
      query    = <<-EOT
        fetch global
        | metric 'custom.googleapis.com/${replace(var.product_id, "-", "_")}${local.metric_suffix}_validation_duration_ms'
        | align delta(1m)
        | group_by [metric.product_id], 1m, [val: percentile(value.${replace(var.product_id, "-", "_")}${local.metric_suffix}_validation_duration_ms, 99)]
        | condition val > 100 'ms'
      EOT
      duration = "300s"
    }
  }

  notification_channels = local.tier2_alert_notification_channels
}

# "Contract Breach": High Error Rate (>20% budget consumption)
resource "google_monitoring_alert_policy" "tier2_high_error_rate" {
  project      = var.project_id
  display_name = "[Tier 2] Gateway High Error Rate - Budget Breach (${var.product_id})${local.name_suffix_display}"
  combiner     = "OR"
  depends_on = [
    google_cloud_run_v2_service.gateway,
    google_monitoring_metric_descriptor.gateway_messages_total
  ]
  conditions {
    display_name = "Error Rate Ratio > 20%"
    condition_monitoring_query_language {
      query    = <<-EOT
        fetch global
        | metric 'custom.googleapis.com/${replace(var.product_id, "-", "_")}${local.metric_suffix}_messages_total'
        | align rate(5m)
        | {
            filter metric.status == 'failed' || metric.status == 'quarantine'
            | group_by [metric.product_id], 5m, [errors: sum(value.${replace(var.product_id, "-", "_")}${local.metric_suffix}_messages_total)]
          ;
            group_by [metric.product_id], 5m, [total: sum(value.${replace(var.product_id, "-", "_")}${local.metric_suffix}_messages_total)]
          }
        | join
        | div
        | condition val() > 0.2
      EOT
      duration = "300s"
    }
  }

  notification_channels = local.tier2_alert_notification_channels
}

# --- CLOUD RUN INFRASTRUCTURE ALERTS ---

# 1. 5xx HTTP Error Rate Alert
resource "google_monitoring_alert_policy" "gateway_5xx_rate" {
  count        = var.enable_infrastructure_alerts ? 1 : 0
  project      = var.project_id
  display_name = "[Tier 1] [INFRA] Gateway Cloud Run 5xx Errors (${var.product_id})${local.name_suffix_display}"
  combiner     = "OR"
  depends_on   = [google_cloud_run_v2_service.gateway]

  conditions {
    display_name = "Cloud Run 5xx Response Rate > 0"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${google_cloud_run_v2_service.gateway.name}\" AND metric.type = \"run.googleapis.com/request_count\" AND metric.label.response_code_class = \"5xx\""
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
      ## [Tier 1] Gateway Cloud Run 5xx Errors Detected
      - **Product ID**: `${var.product_id}`
      - **Service**: `${google_cloud_run_v2_service.gateway.name}`

      ### Triage CLI Command
      ```bash
      gcloud beta run services logs read ${google_cloud_run_v2_service.gateway.name} --region=${var.region} --limit=50
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.all_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# 2. Push/Caller Auth Failure Alert (401/403)
resource "google_monitoring_alert_policy" "gateway_auth_failures" {
  count        = var.enable_infrastructure_alerts ? 1 : 0
  project      = var.project_id
  display_name = "[Tier 1] [INFRA] Gateway Push Invoker Auth 401/403 (${var.product_id})${local.name_suffix_display}"
  combiner     = "OR"
  depends_on   = [google_cloud_run_v2_service.gateway]

  conditions {
    display_name = "Cloud Run 401/403 Auth Failure Rate > 0"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${google_cloud_run_v2_service.gateway.name}\" AND metric.type = \"run.googleapis.com/request_count\" AND metric.label.response_code = one_of(\"401\", \"403\")"
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
      ## [Tier 1] Gateway Authentication/Authorization Rejection
      - **Product ID**: `${var.product_id}`
      - **Service**: `${google_cloud_run_v2_service.gateway.name}`

      ### Triage CLI Command
      ```bash
      gcloud run services get-iam-policy ${google_cloud_run_v2_service.gateway.name} --region=${var.region} --flatten="bindings[].members" --filter="bindings.role:roles/run.invoker"
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.all_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# 3. Container Memory Utilization > 85%
resource "google_monitoring_alert_policy" "gateway_memory_utilization" {
  count        = var.enable_infrastructure_alerts ? 1 : 0
  project      = var.project_id
  display_name = "[Tier 2] [INFRA] Gateway Memory Saturation >85% (${var.product_id})${local.name_suffix_display}"
  combiner     = "OR"
  depends_on   = [google_cloud_run_v2_service.gateway]

  conditions {
    display_name = "Memory Utilization Sustained > 85%"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${google_cloud_run_v2_service.gateway.name}\" AND metric.type = \"run.googleapis.com/container/memory/utilizations\""
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
      ## [Tier 2] Gateway Container Memory Utilization Exceeded
      - **Product ID**: `${var.product_id}`
      - **Memory Threshold**: `${local.active_sla.memory_threshold * 100}%`
      Consider increasing `container_memory` in module configuration.
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.tier2_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# 4. Container CPU Utilization > 85%
resource "google_monitoring_alert_policy" "gateway_cpu_utilization" {
  count        = var.enable_infrastructure_alerts ? 1 : 0
  project      = var.project_id
  display_name = "[Tier 2] [INFRA] Gateway CPU Saturation >85% (${var.product_id})${local.name_suffix_display}"
  combiner     = "OR"
  depends_on   = [google_cloud_run_v2_service.gateway]

  conditions {
    display_name = "CPU Utilization Sustained > 85%"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${google_cloud_run_v2_service.gateway.name}\" AND metric.type = \"run.googleapis.com/container/cpu/utilizations\""
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
      ## [Tier 2] Gateway Container CPU Utilization Exceeded
      - **Product ID**: `${var.product_id}`
      - **CPU Threshold**: `${local.active_sla.cpu_threshold * 100}%`
      Consider increasing `container_cpu` in module configuration.
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.tier2_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# 5. Active Container Instance Ceiling Saturation
resource "google_monitoring_alert_policy" "gateway_instance_saturation" {
  count        = var.enable_infrastructure_alerts ? 1 : 0
  project      = var.project_id
  display_name = "[Tier 2] [INFRA] Gateway Instance Saturation (${var.product_id})${local.name_suffix_display}"
  combiner     = "OR"
  depends_on   = [google_cloud_run_v2_service.gateway]

  conditions {
    display_name = "Active Instances Reached Max Ceiling"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${google_cloud_run_v2_service.gateway.name}\" AND metric.type = \"run.googleapis.com/container/instance_count\" AND metric.label.state = \"active\""
      duration        = "180s"
      comparison      = "COMPARISON_GE"
      threshold_value = var.max_instances
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
      ## [Tier 2] Gateway Active Instance Ceiling Reached
      - **Product ID**: `${var.product_id}`
      - **Max Instances**: `${var.max_instances}`
      Consider increasing `max_instances` in module configuration.
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.tier2_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# --- PUB/SUB PIPELINE & CLEAN BIGQUERY ALERTS ---

# Ingress Push Subscription Latency Alert
resource "google_monitoring_alert_policy" "raw_push_latency" {
  count        = var.enable_infrastructure_alerts ? 1 : 0
  project      = var.project_id
  display_name = "[Tier 1] [INFRA] Raw Push Subscription Latency (${var.product_id})${local.name_suffix_display}"
  combiner     = "OR"
  depends_on   = [google_pubsub_subscription.raw_push]

  conditions {
    display_name = "Raw Push Oldest Unacked Message Age High"
    condition_threshold {
      filter          = "resource.type = \"pubsub_subscription\" AND resource.labels.subscription_id = \"${google_pubsub_subscription.raw_push.name}\" AND metric.type = \"pubsub.googleapis.com/subscription/oldest_unacked_message_age\""
      duration        = "0s"
      comparison      = "COMPARISON_GT"
      threshold_value = local.unacked_age_seconds
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
      ## [Tier 1] Raw Push Subscription Latency Exceeded
      - **Product ID**: `${var.product_id}`
      - **Subscription**: `${google_pubsub_subscription.raw_push.name}`

      ### Triage CLI Command
      ```bash
      gcloud pubsub subscriptions describe ${google_pubsub_subscription.raw_push.name}
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.all_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# Clean-to-BigQuery Subscription Write Stalls (#24)
resource "google_monitoring_alert_policy" "clean_bq_unacked_message_age" {
  for_each     = var.enable_infrastructure_alerts ? var.contracts : {}
  project      = var.project_id
  display_name = "[Tier 1] [INFRA] Clean BigQuery Write Stall (${var.product_id} - ${each.key})${local.name_suffix_display}"
  combiner     = "OR"
  depends_on   = [google_pubsub_subscription.clean_bq]

  conditions {
    display_name = "BigQuery Sink Oldest Unacked Message Age High"
    condition_threshold {
      filter          = "resource.type = \"pubsub_subscription\" AND resource.labels.subscription_id = \"${google_pubsub_subscription.clean_bq[each.key].name}\" AND metric.type = \"pubsub.googleapis.com/subscription/oldest_unacked_message_age\""
      duration        = "0s"
      comparison      = "COMPARISON_GT"
      threshold_value = local.unacked_age_seconds
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
      ## [Tier 1] Clean-to-BigQuery Write Stall
      - **Contract URN**: `${each.value.id}`
      - **Subscription**: `${google_pubsub_subscription.clean_bq[each.key].name}`

      ### Triage CLI Command
      ```bash
      gcloud logging read "resource.type=pubsub_subscription AND resource.labels.subscription_id=${google_pubsub_subscription.clean_bq[each.key].name}" --limit=20
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.all_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# Clean-to-BigQuery Dead-Letter Message Count / Schema Rejections (#24)
resource "google_monitoring_alert_policy" "clean_bq_dead_letter_count" {
  for_each     = var.enable_infrastructure_alerts ? var.contracts : {}
  project      = var.project_id
  display_name = "[Tier 1] [DATA] Clean BigQuery Schema Rejections (${var.product_id} - ${each.key})${local.name_suffix_display}"
  combiner     = "OR"
  depends_on   = [google_pubsub_subscription.clean_bq]

  conditions {
    display_name = "BigQuery Sink Dead Letter Messages Dropped"
    condition_threshold {
      filter          = "resource.type = \"pubsub_subscription\" AND resource.labels.subscription_id = \"${google_pubsub_subscription.clean_bq[each.key].name}\" AND metric.type = \"pubsub.googleapis.com/subscription/dead_letter_message_count\""
      duration        = "0s"
      comparison      = "COMPARISON_GT"
      threshold_value = 0
      trigger {
        count = 1
      }
      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_DELTA"
      }
    }
  }

  documentation {
    content   = <<-EOT
      ## [Tier 1] Clean BigQuery Rejections (Schema/Write Failure)
      - **Contract URN**: `${each.value.id}`
      - **DLQ Subscription**: `${google_pubsub_subscription.dlq_sub.name}`

      ### Triage CLI Command
      ```bash
      gcloud pubsub subscriptions pull ${google_pubsub_subscription.dlq_sub.name} --limit=5 --auto-ack=false
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.all_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

