# Cloud Monitoring Notification Channels

resource "google_monitoring_notification_channel" "slack" {
  count        = var.alert_slack_channel != "" ? 1 : 0
  project      = var.project_id
  display_name = "Slack Alert Channel (${var.product_id})"
  type         = "slack"
  labels = {
    "channel_name" = var.alert_slack_channel
  }
}

resource "google_monitoring_notification_channel" "email" {
  count        = var.alert_email_address != "" ? 1 : 0
  project      = var.project_id
  display_name = "Email Alert Channel (${var.product_id})"
  type         = "email"
  labels = {
    "email_address" = var.alert_email_address
  }
}

# --- METRIC DESCRIPTORS ---

resource "google_monitoring_metric_descriptor" "gateway_messages_total" {
  project      = var.project_id
  description  = "Total count of processed messages"
  display_name = "Gateway Messages Total"
  type         = "custom.googleapis.com/${var.product_id}_messages_total"
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
  type         = "custom.googleapis.com/${var.product_id}_validation_duration_ms"
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
  type         = "custom.googleapis.com/${var.product_id}_validation_errors_total"
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
  display_name = "[Tier 1] Gateway Validation Failure (${var.product_id})"
  combiner     = "OR"
  depends_on = [
    google_cloud_run_v2_service.gateway,
    google_monitoring_metric_descriptor.gateway_validation_errors_total
  ]
  conditions {
    display_name = "Validation Errors > 0"
    condition_threshold {
      filter          = "metric.type=\"custom.googleapis.com/${var.product_id}_validation_errors_total\" resource.type=\"global\""
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

  notification_channels = concat(
    google_monitoring_notification_channel.slack[*].name,
    google_monitoring_notification_channel.email[*].name
  )

  alert_strategy {
    auto_close = "604800s" # 7 days
  }
}

# "Silent Killer": Absence of Data for Tier 1
resource "google_monitoring_alert_policy" "tier1_absence_of_data" {
  project      = var.project_id
  display_name = "[Tier 1] Ingestion Traffic Drop - Absence of Data (${var.product_id})"
  combiner     = "OR"
  depends_on = [
    google_cloud_run_v2_service.gateway,
    google_monitoring_metric_descriptor.gateway_messages_total
  ]
  conditions {
    display_name = "No messages reported"
    condition_absent {
      filter   = "metric.type=\"custom.googleapis.com/${var.product_id}_messages_total\" resource.type=\"global\""
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

  notification_channels = concat(
    google_monitoring_notification_channel.slack[*].name,
    google_monitoring_notification_channel.email[*].name
  )
}

# --- TIER 2: IMPORTANT ---

# Alert on P99 Latency > 100ms
resource "google_monitoring_alert_policy" "tier2_latency_spike" {
  project      = var.project_id
  display_name = "[Tier 2] Gateway Latency Spike P99 > 100ms (${var.product_id})"
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
        | metric 'custom.googleapis.com/${var.product_id}_validation_duration_ms'
        | align delta(1m)
        | group_by [metric.product_id], 1m, [val: percentile(value.${var.product_id}_validation_duration_ms, 99)]
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
  display_name = "[Tier 2] Gateway High Error Rate - Budget Breach (${var.product_id})"
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
        | metric 'custom.googleapis.com/${var.product_id}_messages_total'
        | align rate(5m)
        | {
            filter metric.status == 'failed' || metric.status == 'quarantine'
            | group_by [metric.product_id], 5m, [errors: sum(value.${var.product_id}_messages_total)]
          ;
            group_by [metric.product_id], 5m, [total: sum(value.${var.product_id}_messages_total)]
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
