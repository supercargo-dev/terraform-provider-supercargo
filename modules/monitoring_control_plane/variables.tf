variable "project_id" {
  description = "The GCP project ID where the Control Plane resources reside"
  type        = string
}

variable "environment" {
  description = "The deployment environment (dev, staging, prod)"
  type        = string
  default     = "dev"
}

variable "name_suffix" {
  description = "Optional name suffix for multi-tenant or ephemeral isolation"
  type        = string
  default     = ""
}

variable "region" {
  description = "The GCP region where Cloud Run services and jobs are deployed"
  type        = string
  default     = "europe-west1"
}

# --- Service Identifiers ---

variable "hub_service_name" {
  description = "Cloud Run service name for Hub"
  type        = string
  default     = "hub"
}

variable "vault_service_name" {
  description = "Cloud Run service name for Vault"
  type        = string
  default     = "vault"
}

variable "shovel_service_name" {
  description = "Cloud Run service name for Metadata Shovel"
  type        = string
  default     = "metadata-shovel"
}

variable "mcp_service_name" {
  description = "Cloud Run service name for Supercargo MCP"
  type        = string
  default     = "supercargo-mcp"
}

variable "console_service_name" {
  description = "Cloud Run service name for Supercargo Console"
  type        = string
  default     = "console"
}

variable "analytics_dashboard_service_name" {
  description = "Cloud Run service name for Analytics Dashboard"
  type        = string
  default     = "analytics-dashboard"
}

variable "github_runner_webhook_service_name" {
  description = "Cloud Run service name for GitHub Runner Webhook"
  type        = string
  default     = "github-runner-webhook"
}

# --- Service Max Instance Ceilings ---

variable "hub_max_instances" {
  description = "Max instances configured for Hub Cloud Run service"
  type        = number
  default     = 10
}

variable "vault_max_instances" {
  description = "Max instances configured for Vault Cloud Run service"
  type        = number
  default     = 10
}

variable "shovel_max_instances" {
  description = "Max instances configured for Metadata Shovel Cloud Run service"
  type        = number
  default     = 5
}

variable "mcp_max_instances" {
  description = "Max instances configured for Supercargo MCP Cloud Run service"
  type        = number
  default     = 5
}

variable "console_max_instances" {
  description = "Max instances configured for Console Cloud Run service"
  type        = number
  default     = 5
}

variable "analytics_dashboard_max_instances" {
  description = "Max instances configured for Analytics Dashboard Cloud Run service"
  type        = number
  default     = 5
}

variable "github_runner_webhook_max_instances" {
  description = "Max instances configured for GitHub Runner Webhook Cloud Run service"
  type        = number
  default     = 5
}

# --- Stateful Storage & Outbox Identifiers ---

variable "shovel_trigger_name" {
  description = "Eventarc trigger name for Firestore /outbox events"
  type        = string
  default     = "shovel-trigger"
}

variable "firestore_database_id" {
  description = "Firestore database ID"
  type        = string
  default     = "(default)"
}

variable "kms_key_ring_name" {
  description = "Cloud KMS keyring name for Vault master encryption"
  type        = string
  default     = "supercargo-keyring"
}

variable "kms_crypto_key_name" {
  description = "Cloud KMS crypto key name for Vault master encryption"
  type        = string
  default     = "supercargo-vault-master-key"
}

variable "outbox_audit_subscription_name" {
  description = "Pub/Sub subscription name for outbox audit BigQuery sink (resolves #300)"
  type        = string
  default     = "supercargo-audit-bq"
}

variable "health_events_subscription_name" {
  description = "Pub/Sub subscription name for health events BigQuery sink"
  type        = string
  default     = "supercargo-health-events-bq"
}

variable "events_push_subscription_name" {
  description = "Pub/Sub subscription name for hub events push"
  type        = string
  default     = "hub-events-push"
}

variable "dataform_events_push_subscription_name" {
  description = "Pub/Sub subscription name for Dataform events push"
  type        = string
  default     = "hub-dataform-events-push"
}

variable "dataform_events_dlq_subscription_name" {
  description = "Pub/Sub pull subscription name for Dataform DLQ inspection"
  type        = string
  default     = "dataform-events-dlq-sub"
}

# --- Batch Jobs & Meta-Telemetry Subscriptions ---

variable "analytics_runner_job_name" {
  description = "Cloud Run Job name for Analytics Runner"
  type        = string
  default     = "analytics-runner"
}

variable "github_runner_job_name" {
  description = "Cloud Run Job name for GitHub Self-Hosted Runner"
  type        = string
  default     = "github-runner-job"
}

variable "github_sync_job_names" {
  description = "List of Cloud Run Job names for GitHub metadata sync"
  type        = list(string)
  default = [
    "analytics-github-sync-repositories",
    "analytics-github-sync-issues",
    "analytics-github-sync-pulls",
    "analytics-github-sync-workflow-runs",
    "analytics-github-sync-billing",
  ]
}

variable "scheduler_job_names" {
  description = "List of Cloud Scheduler job names for control plane triggers"
  type        = list(string)
  default = [
    "analytics-runner-schedule",
    "analytics-github-sync-repositories-schedule",
    "analytics-github-sync-issues-schedule",
    "analytics-github-sync-pulls-schedule",
    "analytics-github-sync-workflow-runs-schedule",
    "analytics-github-sync-billing-schedule",
  ]
}

variable "telemetry_subscriptions" {
  description = "List of operational telemetry BigQuery streaming subscriptions monitored for lag"
  type        = list(string)
  default = [
    "bigquery-hub-operations",
    "bigquery-vault-operations",
    "bigquery-gateway-executions",
    "bigquery-mcp-interactions",
    "bigquery-sentinel-interventions",
    "bigquery-platform-telemetry",
    "bigquery-flusher-heartbeat",
  ]
}

variable "telemetry_dlq_subscription_name" {
  description = "Pub/Sub subscription name for platform telemetry DLQ"
  type        = string
  default     = "supercargo-telemetry-dlq"
}

variable "monitored_batch_jobs" {
  description = "Optional explicit list of Cloud Run Job names to monitor. If null, derived from individual job name variables."
  type        = list(string)
  default     = null
}

# --- SLA Tiering & Alert Policies ---

variable "sla_tier" {
  description = "The SLA tier for the control plane (TIER_1, TIER_2, TIER_3). Calibrates alert thresholds and paging severity."
  type        = string
  default     = "TIER_1"
  validation {
    condition     = contains(["TIER_1", "TIER_2", "TIER_3"], var.sla_tier)
    error_message = "The sla_tier variable must be one of: TIER_1, TIER_2, TIER_3."
  }
}

variable "enable_control_plane_alerts" {
  description = "Whether to provision Cloud Monitoring alert policies for control plane services and pipelines"
  type        = bool
  default     = true
}

variable "enable_slos" {
  description = "Whether to provision Custom Services, SLOs, and burn-rate alert policies"
  type        = bool
  default     = true
}

variable "enable_dashboards" {
  description = "Whether to provision the turnkey persona dashboards"
  type        = bool
  default     = true
}

variable "alert_notification_channels" {
  description = "List of Cloud Monitoring notification channel IDs for alert routing"
  type        = list(string)
  default     = []
}

variable "additional_sre_notification_channels" {
  description = "Additional notification channels for SRE operations (webhooks, email)"
  type        = list(string)
  default     = []
}

variable "alert_slack_channel" {
  description = "Slack notification channel name (e.g. #supercargo-sre)"
  type        = string
  default     = ""
}

variable "alert_email_address" {
  description = "Email notification address for alerts"
  type        = string
  default     = ""
}

variable "alert_pagerduty_service_key" {
  description = "PagerDuty integration service key for Tier 1 paging"
  type        = string
  default     = ""
  sensitive   = true
}

variable "unacked_message_age_threshold_seconds" {
  description = "Optional override for Pub/Sub unacknowledged message age alert threshold. If unset, derived from sla_tier."
  type        = number
  default     = null
}
