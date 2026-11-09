locals {
  name_suffix_display = var.name_suffix != "" ? " - ${var.name_suffix}" : ""
}

# DLT Cloud Run Job Execution Failure Alert
resource "google_monitoring_alert_policy" "dlt_job_failed_execution" {
  for_each     = var.enable_infrastructure_alerts ? local.dlt_input_ports : {}
  project      = var.project_id
  display_name = "[Tier 1] [INFRA] DLT Batch Ingestion Failed (${local.product_id} - ${each.key})${local.name_suffix_display}"
  combiner     = "OR"
  depends_on   = [google_cloud_run_v2_job.dlt_jobs]

  conditions {
    display_name = "DLT Job Completed With Failure"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_job\" AND resource.labels.job_name = \"${google_cloud_run_v2_job.dlt_jobs[each.key].name}\" AND metric.type = \"run.googleapis.com/job/completed_execution_count\" AND metric.label.result = \"failed\""
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
      ## [Tier 1] DLT Batch Ingestion Job Failed
      - **Product ID**: `${local.product_id}`
      - **Port Name**: `${each.key}`
      - **Job Name**: `${google_cloud_run_v2_job.dlt_jobs[each.key].name}`

      ### Triage CLI Command
      ```bash
      gcloud run jobs executions list --job=${google_cloud_run_v2_job.dlt_jobs[each.key].name} --region=${var.region} --limit=5
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = module.gateway.alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# DLT Cloud Scheduler Trigger Failure Alert
resource "google_monitoring_alert_policy" "dlt_scheduler_failure" {
  for_each     = var.enable_infrastructure_alerts ? local.dlt_input_ports : {}
  project      = var.project_id
  display_name = "[Tier 1] [INFRA] DLT Scheduler Trigger Failure (${local.product_id} - ${each.key})${local.name_suffix_display}"
  combiner     = "OR"
  depends_on   = [google_cloud_scheduler_job.dlt_schedules]

  conditions {
    display_name = "Scheduler Job Non-Success Status"
    condition_matched_log {
      filter = "resource.type = \"cloud_scheduler_job\" AND resource.labels.job_id = \"${google_cloud_scheduler_job.dlt_schedules[each.key].name}\" AND severity >= ERROR"
    }
  }

  documentation {
    content   = <<-EOT
      ## [Tier 1] DLT Cloud Scheduler Trigger Failure
      - **Product ID**: `${local.product_id}`
      - **Scheduler Job**: `${google_cloud_scheduler_job.dlt_schedules[each.key].name}`

      ### Triage CLI Command
      ```bash
      gcloud logging read "resource.type=cloud_scheduler_job AND resource.labels.job_id=${google_cloud_scheduler_job.dlt_schedules[each.key].name}" --limit=10
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = module.gateway.alert_notification_channels
  alert_strategy {
    notification_rate_limit {
      period = "300s"
    }
    auto_close = "604800s"
  }
}

# Input Push Subscription Latency Alert
resource "google_monitoring_alert_policy" "input_push_latency" {
  for_each     = var.enable_infrastructure_alerts ? local.pubsub_input_ports : {}
  project      = var.project_id
  display_name = "[Tier 2] [INFRA] Input Push Subscription Latency (${local.product_id} - ${each.key})${local.name_suffix_display}"
  combiner     = "OR"
  depends_on   = [google_pubsub_subscription.input_push]

  conditions {
    display_name = "Input Push Oldest Unacked Message Age High"
    condition_threshold {
      filter          = "resource.type = \"pubsub_subscription\" AND resource.labels.subscription_id = \"${google_pubsub_subscription.input_push[each.key].name}\" AND metric.type = \"pubsub.googleapis.com/subscription/oldest_unacked_message_age\""
      duration        = "0s"
      comparison      = "COMPARISON_GT"
      threshold_value = module.gateway.unacked_message_age_threshold_seconds
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
      ## [Tier 2] Input Push Subscription Latency Exceeded
      - **Product ID**: `${local.product_id}`
      - **Port Name**: `${each.key}`
      - **Subscription**: `${google_pubsub_subscription.input_push[each.key].name}`
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = module.gateway.tier2_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}
