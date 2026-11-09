# --- Slug & Display Name Hygiene (Whitespace Elimination) ---
locals {
  name_suffix_id      = var.name_suffix != null && var.name_suffix != "" ? "-${var.name_suffix}" : ""
  name_suffix_display = var.name_suffix != null && var.name_suffix != "" ? " - ${var.name_suffix}" : ""
}

# --- Cloud Monitoring Notification Channels ---

resource "google_monitoring_notification_channel" "slack" {
  count        = var.alert_slack_channel != null && var.alert_slack_channel != "" ? 1 : 0
  project      = var.project_id
  display_name = "Slack Alert Channel (Control Plane)${local.name_suffix_display}"
  type         = "slack"
  labels = {
    "channel_name" = var.alert_slack_channel
  }
}

resource "google_monitoring_notification_channel" "email" {
  count        = var.alert_email_address != null && var.alert_email_address != "" ? 1 : 0
  project      = var.project_id
  display_name = "Email Alert Channel (Control Plane)${local.name_suffix_display}"
  type         = "email"
  labels = {
    "email_address" = var.alert_email_address
  }
}

resource "google_monitoring_notification_channel" "pagerduty" {
  count        = var.alert_pagerduty_service_key != null && var.alert_pagerduty_service_key != "" ? 1 : 0
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

  # Core Pub/Sub Subscriptions Monitored for Lag with Defensive Name Sanitization
  raw_core_subscriptions = {
    outbox_audit_bq = {
      name = var.outbox_audit_subscription_name != null && var.outbox_audit_subscription_name != "" ? element(split("/", var.outbox_audit_subscription_name), length(split("/", var.outbox_audit_subscription_name)) - 1) : ""
      desc = "Outbox Audit BigQuery Sink (Issue #300)"
      tier = "Tier 1"
    }
    health_events_bq = {
      name = var.health_events_subscription_name != null && var.health_events_subscription_name != "" ? element(split("/", var.health_events_subscription_name), length(split("/", var.health_events_subscription_name)) - 1) : ""
      desc = "Health Events BigQuery Sink"
      tier = "Tier 1"
    }
    events_push = {
      name = var.events_push_subscription_name != null && var.events_push_subscription_name != "" ? element(split("/", var.events_push_subscription_name), length(split("/", var.events_push_subscription_name)) - 1) : ""
      desc = "Hub Alert Relay Events Push"
      tier = "Tier 1"
    }
    dataform_events_push = {
      name = var.dataform_events_push_subscription_name != null && var.dataform_events_push_subscription_name != "" ? element(split("/", var.dataform_events_push_subscription_name), length(split("/", var.dataform_events_push_subscription_name)) - 1) : ""
      desc = "Dataform Events Push"
      tier = "Tier 2"
    }
  }

  core_subscriptions = {
    for k, v in local.raw_core_subscriptions : k => v if v.name != null && v.name != ""
  }

  # Monitored Cloud Run Batch Jobs
  monitored_batch_jobs = var.monitored_batch_jobs != null ? toset(compact(var.monitored_batch_jobs)) : toset(compact(concat(
    [var.analytics_runner_job_name, var.github_runner_job_name],
    var.github_sync_job_names
  )))
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
      comparison      = "COMPARISON_GT"
      threshold_value = each.value.max_instances - 1
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

# --- Firestore Contention & Quota Alerts (Strict Database ID Scoping) ---

resource "google_monitoring_alert_policy" "firestore_aborted_transactions" {
  count        = var.enable_control_plane_alerts ? 1 : 0
  project      = var.project_id
  display_name = "[Tier 1] [DATA] Firestore Transaction Contention (ABORTED)${local.name_suffix_display}"
  combiner     = "OR"

  conditions {
    display_name = "Firestore ABORTED Contention Rate > 0"
    condition_threshold {
      filter          = "resource.type = \"firestore_database\" AND resource.labels.database_id = \"${var.firestore_database_id}\" AND metric.type = \"firestore.googleapis.com/api/request_count\" AND metric.label.response_code = \"ABORTED\""
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
      ## [Tier 1] Firestore Transaction Contention (ABORTED)
      High rate of concurrent write conflicts detected on database `${var.firestore_database_id}`.

      ### Triage CLI Command
      ```bash
      gcloud logging read "resource.type=firestore_database AND resource.labels.database_id=${var.firestore_database_id} AND protoPayload.status.code=10" --limit=20
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.all_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

resource "google_monitoring_alert_policy" "firestore_quota_exhaustion" {
  count        = var.enable_control_plane_alerts ? 1 : 0
  project      = var.project_id
  display_name = "[Tier 1] [INFRA] Firestore Quota Exhaustion (RESOURCE_EXHAUSTED)${local.name_suffix_display}"
  combiner     = "OR"

  conditions {
    display_name = "Firestore Quota Exhaustion Rate > 0"
    condition_threshold {
      filter          = "resource.type = \"firestore_database\" AND resource.labels.database_id = \"${var.firestore_database_id}\" AND metric.type = \"firestore.googleapis.com/api/request_count\" AND metric.label.response_code = \"RESOURCE_EXHAUSTED\""
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
      ## [Tier 1] Firestore Quota Exhaustion Detected
      Firestore operations rejected due to RESOURCE_EXHAUSTED on `${var.firestore_database_id}`.

      ### Triage CLI Command
      ```bash
      gcloud logging read "resource.type=firestore_database AND resource.labels.database_id=${var.firestore_database_id} AND protoPayload.status.code=8" --limit=20
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.all_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# --- Eventarc Shovel Trigger Delivery Failures ---

resource "google_monitoring_alert_policy" "shovel_trigger_delivery_failures" {
  count        = var.enable_control_plane_alerts ? 1 : 0
  project      = var.project_id
  display_name = "[Tier 1] [INFRA] Eventarc Shovel Trigger Delivery Failures${local.name_suffix_display}"
  combiner     = "OR"

  conditions {
    display_name = "Eventarc Shovel Trigger Failed Deliveries > 0"
    condition_threshold {
      filter          = "resource.type = \"eventarc_trigger\" AND resource.labels.trigger_id = \"${var.shovel_trigger_name}\" AND metric.type = \"eventarc.googleapis.com/delivery_attempt_count\" AND metric.label.response_code != \"OK\""
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
      ## [Tier 1] Eventarc Shovel Trigger Delivery Failures
      - **Trigger**: `${var.shovel_trigger_name}`

      ### Triage CLI Command
      ```bash
      gcloud eventarc triggers describe ${var.shovel_trigger_name} --location=${var.region}
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.all_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# --- Core Pub/Sub Subscriptions Unacked Message Age (Resolves #300) ---

resource "google_monitoring_alert_policy" "core_subscription_unacked_age" {
  for_each     = var.enable_control_plane_alerts ? local.core_subscriptions : {}
  project      = var.project_id
  display_name = "[${each.value.tier}] [INFRA] Subscription Lag: ${each.value.desc}${local.name_suffix_display}"
  combiner     = "OR"

  conditions {
    display_name = "Subscription Oldest Unacked Message Age High"
    condition_threshold {
      filter          = "resource.type = \"pubsub_subscription\" AND resource.labels.subscription_id = \"${each.value.name}\" AND metric.type = \"pubsub.googleapis.com/subscription/oldest_unacked_message_age\""
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
      ## Subscription Backlog Latency Exceeded
      - **Subscription**: `${each.value.name}`
      - **Description**: `${each.value.desc}`

      ### Triage Runbook
      BigQuery push subscriptions cannot be pulled directly. Check Cloud Logging:
      ```bash
      gcloud logging read "resource.type=pubsub_subscription AND resource.labels.subscription_id=${each.value.name}" --limit=20
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = each.value.tier == "Tier 1" ? local.all_alert_notification_channels : local.tier2_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# --- Dataform DLQ Backlog Alert ---

resource "google_monitoring_alert_policy" "dataform_dlq_backlog" {
  count        = var.enable_control_plane_alerts && var.dataform_events_dlq_subscription_name != null && var.dataform_events_dlq_subscription_name != "" ? 1 : 0
  project      = var.project_id
  display_name = "[Tier 1] [DATA] Dataform Dead Letter Queue Backlog${local.name_suffix_display}"
  combiner     = "OR"

  conditions {
    display_name = "Dataform DLQ Undelivered Messages > 0"
    condition_threshold {
      filter          = "resource.type = \"pubsub_subscription\" AND resource.labels.subscription_id = \"${var.dataform_events_dlq_subscription_name}\" AND metric.type = \"pubsub.googleapis.com/subscription/num_undelivered_messages\""
      duration        = "0s"
      comparison      = "COMPARISON_GT"
      threshold_value = 0
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
      ## [Tier 1] Dataform Dead Letter Queue Messages Dropped
      - **Subscription**: `${var.dataform_events_dlq_subscription_name}`

      ### Triage CLI Command
      ```bash
      gcloud pubsub subscriptions pull ${var.dataform_events_dlq_subscription_name} --limit=5 --auto-ack=false
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.all_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# --- Cloud KMS Crypto Request Rejections Alert ---

resource "google_monitoring_alert_policy" "kms_access_denials" {
  count        = var.enable_control_plane_alerts ? 1 : 0
  project      = var.project_id
  display_name = "[Tier 1] [SECURITY] Cloud KMS Crypto Request Rejections${local.name_suffix_display}"
  combiner     = "OR"

  conditions {
    display_name = "Cloud KMS Rejection Rate > 0"
    condition_threshold {
      filter          = "resource.type = \"cloudkms_cryptokey\" AND metric.type = \"cloudkms.googleapis.com/crypto_requests\" AND metric.label.response_code != \"OK\""
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
      ## [Tier 1] Cloud KMS Cryptographic Request Rejection
      Key operations failing. Verify KMS master key status and IAM roles for Vault SA.

      ### Triage CLI Command
      ```bash
      gcloud kms keys describe ${var.kms_crypto_key_name} --keyring=${var.kms_key_ring_name} --location=${var.region}
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.all_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# --- Vault 403 Forbidden Calls Spike Alert ---

resource "google_monitoring_alert_policy" "vault_forbidden_spikes" {
  count        = var.enable_control_plane_alerts && var.vault_service_name != null && var.vault_service_name != "" ? 1 : 0
  project      = var.project_id
  display_name = "[Tier 1] [SECURITY] Spikes in Forbidden (403) Vault Calls${local.name_suffix_display}"
  combiner     = "OR"

  conditions {
    display_name = "Vault 403 Rate > 0"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND resource.labels.service_name = \"${var.vault_service_name}\" AND metric.type = \"run.googleapis.com/request_count\" AND metric.label.response_code = \"403\""
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
      ## [Tier 1] Spikes in Forbidden (403) Vault Calls
      Unauthorized callers attempting tokenization or key access on Vault.

      ### Triage CLI Command
      ```bash
      gcloud beta run services logs read ${var.vault_service_name} --region=${var.region} --filter=\"httpRequest.status=403\" --limit=50
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.all_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# --- Centralized Batch Jobs, Schedulers & Meta-Telemetry BigQuery Alerts ---

# 1. Cloud Run Job Execution Failure Alert
resource "google_monitoring_alert_policy" "job_failed_execution" {
  for_each     = var.enable_control_plane_alerts ? local.monitored_batch_jobs : toset([])
  project      = var.project_id
  display_name = "[Tier 1] [INFRA] Cloud Run Job Execution Failure (${each.value})${local.name_suffix_display}"
  combiner     = "OR"

  conditions {
    display_name = "Cloud Run Job Completed with Failure"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_job\" AND resource.labels.job_name = \"${each.value}\" AND metric.type = \"run.googleapis.com/job/completed_execution_count\" AND metric.label.result = \"failed\""
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
      ## [Tier 1] Cloud Run Job Execution Failure
      - **Job Name**: `${each.value}`

      ### Triage CLI Command
      ```bash
      gcloud run jobs executions list --job=${each.value} --region=${var.region} --limit=5
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.all_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# 2. Cloud Scheduler Dispatch Failure Alert
resource "google_monitoring_alert_policy" "scheduler_dispatch_failure" {
  for_each     = var.enable_control_plane_alerts ? toset(var.scheduler_job_names) : toset([])
  project      = var.project_id
  display_name = "[Tier 1] [INFRA] Cloud Scheduler Dispatch Failure (${each.value})${local.name_suffix_display}"
  combiner     = "OR"

  conditions {
    display_name = "Cloud Scheduler HTTP Status != SUCCESS"
    condition_threshold {
      filter          = "resource.type = \"cloud_scheduler_job\" AND resource.labels.job_id = \"${each.value}\" AND metric.type = \"cloudscheduler.googleapis.com/job/attempt_count\" AND metric.label.response_status != \"SUCCESS\""
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
      ## [Tier 1] Cloud Scheduler Dispatch Failure
      - **Scheduler Job**: `${each.value}`

      ### Triage CLI Command
      ```bash
      gcloud logging read "resource.type=cloud_scheduler_job AND resource.labels.job_id=${each.value}" --limit=10
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.all_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# 3. Operational Meta-Telemetry BigQuery Subscriptions Lag Alert
resource "google_monitoring_alert_policy" "telemetry_bq_unacked_age" {
  for_each     = var.enable_control_plane_alerts ? toset(var.telemetry_subscriptions) : toset([])
  project      = var.project_id
  display_name = "[Tier 1] [INFRA] Meta-Telemetry Subscription Lag (${each.value})${local.name_suffix_display}"
  combiner     = "OR"

  conditions {
    display_name = "Telemetry BQ Subscription Oldest Unacked Message Age High"
    condition_threshold {
      filter          = "resource.type = \"pubsub_subscription\" AND resource.labels.subscription_id = \"${each.value}\" AND metric.type = \"pubsub.googleapis.com/subscription/oldest_unacked_message_age\""
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
      ## [Tier 1] Operational Telemetry BigQuery Subscription Lag
      - **Subscription**: `${each.value}`

      ### Triage CLI Command
      ```bash
      gcloud logging read "resource.type=pubsub_subscription AND resource.labels.subscription_id=${each.value}" --limit=20
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.all_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}

# 4. Meta-Telemetry Dead Letter Queue Backlog Alert
resource "google_monitoring_alert_policy" "telemetry_dlq_backlog" {
  count        = var.enable_control_plane_alerts && var.telemetry_dlq_subscription_name != null && var.telemetry_dlq_subscription_name != "" ? 1 : 0
  project      = var.project_id
  display_name = "[Tier 1] [DATA] Meta-Telemetry Dead Letter Queue Backlog${local.name_suffix_display}"
  combiner     = "OR"

  conditions {
    display_name = "Telemetry DLQ Undelivered Messages > 0"
    condition_threshold {
      filter          = "resource.type = \"pubsub_subscription\" AND resource.labels.subscription_id = \"${var.telemetry_dlq_subscription_name}\" AND metric.type = \"pubsub.googleapis.com/subscription/num_undelivered_messages\""
      duration        = "0s"
      comparison      = "COMPARISON_GT"
      threshold_value = 0
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
      ## [Tier 1] Meta-Telemetry Dead Letter Queue Backlog Detected
      - **DLQ Subscription**: `${var.telemetry_dlq_subscription_name}`

      ### Triage CLI Command
      ```bash
      gcloud pubsub subscriptions pull ${var.telemetry_dlq_subscription_name} --limit=5 --auto-ack=false
      ```
    EOT
    mime_type = "text/markdown"
  }

  notification_channels = local.all_alert_notification_channels
  alert_strategy {
    auto_close = "604800s"
  }
}
