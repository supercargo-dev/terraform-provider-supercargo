# Cloud Monitoring Notification Channels

resource "google_monitoring_notification_channel" "slack" {
  count        = var.alert_slack_channel != "" ? 1 : 0
  project      = var.project_id
  display_name = "Slack Alert Channel (${var.product_id}) - ${random_id.suffix.hex}"
  type         = "slack"
  labels = {
    "channel_name" = var.alert_slack_channel
  }
  depends_on = [time_sleep.wait_for_gateway_apis]
}

resource "google_monitoring_notification_channel" "email" {
  count        = var.alert_email_address != "" ? 1 : 0
  project      = var.project_id
  display_name = "Email Alert Channel (${var.product_id}) - ${random_id.suffix.hex}"
  type         = "email"
  labels = {
    "email_address" = var.alert_email_address
  }
  depends_on = [time_sleep.wait_for_gateway_apis]
}

resource "google_monitoring_notification_channel" "pagerduty" {
  count        = var.alert_pagerduty_service_key != "" ? 1 : 0
  project      = var.project_id
  display_name = "PagerDuty Alert Channel (${var.product_id}) - ${random_id.suffix.hex}"
  type         = "pagerduty"
  sensitive_labels {
    service_key = var.alert_pagerduty_service_key
  }
  depends_on = [time_sleep.wait_for_gateway_apis]
}

locals {
  all_alert_notification_channels = distinct(compact(concat(
    google_monitoring_notification_channel.slack[*].name,
    google_monitoring_notification_channel.email[*].name,
    google_monitoring_notification_channel.pagerduty[*].name,
    var.alert_notification_channels
  )))
  monitored_contracts_summary = length(var.contracts) > 0 ? join(", ", [for k, v in var.contracts : v.id]) : var.product_id
}

# --- METRIC DESCRIPTORS ---

resource "google_monitoring_metric_descriptor" "gateway_messages_total" {
  project      = var.project_id
  description  = "Total count of processed messages"
  display_name = "Gateway Messages Total"
  type         = "custom.googleapis.com/${replace(var.product_id, "-", "_")}_${random_id.suffix.hex}_messages_total"
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
  type         = "custom.googleapis.com/${replace(var.product_id, "-", "_")}_${random_id.suffix.hex}_validation_duration_ms"
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
  type         = "custom.googleapis.com/${replace(var.product_id, "-", "_")}_${random_id.suffix.hex}_validation_errors_total"
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
  display_name = "[Tier 1] Gateway Validation Failure (${var.product_id}) - ${random_id.suffix.hex}"
  combiner     = "OR"
  depends_on = [
    google_cloud_run_v2_service.gateway,
    google_monitoring_metric_descriptor.gateway_validation_errors_total
  ]
  conditions {
    display_name = "Validation Errors > 0"
    condition_threshold {
      filter          = "metric.type=\"custom.googleapis.com/${replace(var.product_id, "-", "_")}_${random_id.suffix.hex}_validation_errors_total\" resource.type=\"global\""
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
  display_name = "[Tier 1] Ingestion Traffic Drop - Absence of Data (${var.product_id}) - ${random_id.suffix.hex}"
  combiner     = "OR"
  depends_on = [
    google_cloud_run_v2_service.gateway,
    google_monitoring_metric_descriptor.gateway_messages_total
  ]
  conditions {
    display_name = "No messages reported"
    condition_absent {
      filter   = "metric.type=\"custom.googleapis.com/${replace(var.product_id, "-", "_")}_${random_id.suffix.hex}_messages_total\" resource.type=\"global\""
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
  display_name = "[Tier 1] DLQ Quarantined Messages Backlog (${var.product_id}) - ${random_id.suffix.hex}"
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
  display_name = "[Tier 1] DLQ Message Latency Exceeded (${var.product_id}) - ${random_id.suffix.hex}"
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
  display_name = "[Tier 2] Gateway Latency Spike P99 > 100ms (${var.product_id}) - ${random_id.suffix.hex}"
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
        | metric 'custom.googleapis.com/${replace(var.product_id, "-", "_")}_${random_id.suffix.hex}_validation_duration_ms'
        | align delta(1m)
        | group_by [metric.product_id], 1m, [val: percentile(value.${replace(var.product_id, "-", "_")}_${random_id.suffix.hex}_validation_duration_ms, 99)]
        | condition val > 100 'ms'
      EOT
      duration = "300s"
    }
  }

  notification_channels = google_monitoring_notification_channel.slack[*].name
}

# "Contract Breach": High Error Rate (>20% budget consumption)
resource "google_monitoring_alert_policy" "tier2_high_error_rate" {
  project      = var.project_id
  display_name = "[Tier 2] Gateway High Error Rate - Budget Breach (${var.product_id}) - ${random_id.suffix.hex}"
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
        | metric 'custom.googleapis.com/${replace(var.product_id, "-", "_")}_${random_id.suffix.hex}_messages_total'
        | align rate(5m)
        | {
            filter metric.status == 'failed' || metric.status == 'quarantine'
            | group_by [metric.product_id], 5m, [errors: sum(value.${replace(var.product_id, "-", "_")}_${random_id.suffix.hex}_messages_total)]
          ;
            group_by [metric.product_id], 5m, [total: sum(value.${replace(var.product_id, "-", "_")}_${random_id.suffix.hex}_messages_total)]
          }
        | join
        | div
        | condition val() > 0.2
      EOT
      duration = "300s"
    }
  }

  notification_channels = google_monitoring_notification_channel.slack[*].name
}
