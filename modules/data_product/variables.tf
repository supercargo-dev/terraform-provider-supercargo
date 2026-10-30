variable "manifest_file" {
  description = "Path to the Data Product manifest YAML/JSON file"
  type        = string
}

variable "project_id" {
  description = "The GCP project ID"
  type        = string
}

variable "region" {
  description = "The GCP region"
  type        = string
}

variable "contracts" {
  description = "Map of contracts to register and provision"
  type = map(object({
    version      = string
    schema_json  = string
    commit_sha   = string
    content_hash = string
  }))
  default = {}
}

variable "image" {
  description = "The container image for The Guardian service"
  type        = string
}

variable "log_level" {
  description = "Log level for the service"
  type        = string
  default     = "info"
}

variable "container_memory" {
  description = "The memory limit for the container."
  type        = string
  default     = "512Mi"
}

variable "container_cpu" {
  description = "The CPU limit for the container."
  type        = string
  default     = "1"
}

variable "pubsub_max_delivery_attempts" {
  description = "The maximum number of delivery attempts for a message."
  type        = number
  default     = 5
}

variable "pubsub_minimum_backoff" {
  description = "The minimum backoff duration for retries."
  type        = string
  default     = "30s"
}

variable "pubsub_maximum_backoff" {
  description = "The maximum backoff duration for retries."
  type        = string
  default     = "600s"
}

variable "pubsub_message_retention_duration" {
  description = "The duration to retain messages in subscriptions."
  type        = string
  default     = "600s" # 10 minutes
}

variable "pubsub_expiration_policy_ttl" {
  description = "The TTL for idle subscriptions to auto-delete."
  type        = string
  default     = "1209600s" # 14 days
}

variable "hub_address" {
  description = "The address of the Hub service"
  type        = string
}

variable "kms_address" {
  description = "The address of the KMS (Vault) service"
  type        = string
}

variable "master_key_uri" {
  description = "The URI of the Master Key in GCP KMS"
  type        = string
}

variable "hub_iap_client_id" {
  description = "The IAP Client ID for the Hub service"
  type        = string
  default     = ""
}

variable "vault_iap_client_id" {
  description = "The IAP Client ID for the Vault service"
  type        = string
  default     = ""
}

variable "hub_oidc_audience" {
  description = "The OIDC audience for the Hub service (Internal OIDC)"
  type        = string
  default     = ""
}

variable "vault_oidc_audience" {
  description = "The OIDC audience for the Vault service (Internal OIDC)"
  type        = string
  default     = ""
}

variable "bigquery_dataset_id" {
  description = "The BigQuery dataset ID where the table should be created"
  type        = string
  default     = ""
}

variable "auth_enforce" {
  description = "Whether to enforce OIDC authentication (Shadow Mode if false)"
  type        = bool
  default     = true
}

variable "force_deploy_trigger" {
  description = "A value that, when changed, triggers a new Cloud Run revision (e.g. git SHA or image digest). Use 'latest' or a fixed value for idempotency."
  type        = string
  default     = "static"
}
variable "custom_audiences" {
  type    = list(string)
  default = []
}

variable "gateway_audience" {
  type    = string
  default = ""
}

variable "bigquery_deletion_protection" {
  description = "Whether to enable deletion protection on BigQuery tables"
  type        = bool
  default     = true
}

variable "authorized_invokers" {
  description = "List of IAM members authorized to invoke the gateway"
  type        = list(string)
  default     = []
}

variable "authorized_invoker_service_accounts" {
  description = "List of service account emails authorized to invoke the gateway service"
  type        = list(string)
  default     = []
}

variable "enable_dlq_alerts" {
  description = "Whether to enable Cloud Monitoring alert policies for DLQ"
  type        = bool
  default     = true
}

variable "dlq_alert_threshold" {
  description = "Threshold count of undelivered messages in DLQ to trigger alert"
  type        = number
  default     = 0
}

variable "dlq_unacked_message_age_seconds" {
  description = "Threshold age (in seconds) of oldest unacknowledged message in DLQ to trigger alert"
  type        = number
  default     = 300
}

variable "dlq_runbook_url" {
  description = "Runbook URL for DLQ incident remediation"
  type        = string
  default     = "https://docs.supercargo.dev/operations/runbooks/dlq-remediation"
}

variable "alert_slack_channel" {
  description = "Slack channel for alerts (e.g. #team-alerts)"
  type        = string
  default     = ""
}

variable "alert_email_address" {
  description = "Email address for alerts"
  type        = string
  default     = ""
}

variable "alert_pagerduty_service_key" {
  description = "PagerDuty integration/service key for alert routing"
  type        = string
  default     = ""
  sensitive   = true
}

variable "alert_notification_channels" {
  description = "List of existing Cloud Monitoring notification channel IDs to attach to alerts"
  type        = list(string)
  default     = []
}

variable "name_suffix" {
  description = "Optional deterministic suffix appended to resource names for environment/branch disambiguation. Defaults to empty."
  type        = string
  default     = ""
}

variable "service_account_id" {
  description = "Optional custom service account ID for the Gateway. If unset, defaults to 'gateway-<product_id>' with optional name_suffix."
  type        = string
  default     = ""
  validation {
    condition     = var.service_account_id == "" || (length(var.service_account_id) >= 6 && length(var.service_account_id) <= 30 && can(regex("^[a-z]([-a-z0-9]*[a-z0-9])$", var.service_account_id)))
    error_message = "The service_account_id must be between 6 and 30 characters and conform to GCP service account naming conventions."
  }
}

variable "dlt_runner_image" {
  description = "The container image for DLT pipeline runners"
  type        = string
  default     = "ghcr.io/supercargo-dev/supercargo-dlt-runner:latest"
}

variable "dlt_deletion_protection" {
  description = "Whether to protect DLT BigQuery tables from accidental deletion"
  type        = bool
  default     = false
}

variable "dlt_time_zone" {
  description = "Time zone for Cloud Scheduler cron jobs (e.g. UTC, America/New_York)"
  type        = string
  default     = "UTC"
}

variable "dlt_cpu" {
  description = "The CPU allocation for DLT Cloud Run jobs"
  type        = string
  default     = "1"
}

variable "dlt_memory" {
  description = "The memory limit for DLT Cloud Run jobs"
  type        = string
  default     = "1Gi"
}

variable "dlt_timeout" {
  description = "The maximum execution timeout for DLT Cloud Run jobs"
  type        = string
  default     = "3600s"
}

variable "dlt_max_retries" {
  description = "The maximum number of retry attempts for failed DLT Cloud Run jobs"
  type        = number
  default     = 3
}

variable "dlt_runner_service_account_id" {
  description = "Optional custom service account ID for DLT runner Cloud Run jobs. If unset, derived deterministically."
  type        = string
  default     = ""
  validation {
    condition     = var.dlt_runner_service_account_id == "" || (length(var.dlt_runner_service_account_id) >= 6 && length(var.dlt_runner_service_account_id) <= 30 && can(regex("^[a-z]([-a-z0-9]*[a-z0-9])$", var.dlt_runner_service_account_id)))
    error_message = "The dlt_runner_service_account_id must be between 6 and 30 characters and conform to GCP service account naming conventions."
  }
}

variable "dlt_scheduler_service_account_id" {
  description = "Optional custom service account ID for DLT Cloud Scheduler jobs. If unset, derived deterministically."
  type        = string
  default     = ""
  validation {
    condition     = var.dlt_scheduler_service_account_id == "" || (length(var.dlt_scheduler_service_account_id) >= 6 && length(var.dlt_scheduler_service_account_id) <= 30 && can(regex("^[a-z]([-a-z0-9]*[a-z0-9])$", var.dlt_scheduler_service_account_id)))
    error_message = "The dlt_scheduler_service_account_id must be between 6 and 30 characters and conform to GCP service account naming conventions."
  }
}

variable "dlt_scheduler_attempt_deadline" {
  description = "The attempt deadline for Cloud Scheduler jobs triggering DLT Cloud Run jobs"
  type        = string
  default     = "320s"
}



