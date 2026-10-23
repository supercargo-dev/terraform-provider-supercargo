variable "project_id" {
  description = "The Google Cloud project ID where resources will be provisioned."
  type        = string
}

variable "region" {
  description = "The Google Cloud region for the Cloud Run Job and Cloud Scheduler."
  type        = string
  default     = "europe-west1"
}

variable "job_name" {
  description = "Name of the Cloud Run v2 Job."
  type        = string
  default     = "analytics-github-sync"
}

variable "container_image" {
  description = "Container image URI for the GitHub sync job."
  type        = string
}

variable "org" {
  description = "GitHub organization name for repository discovery and ingestion."
  type        = string
}

variable "topic" {
  description = "GitHub topic for autonomous repository discovery."
  type        = string
  default     = "supercargo-analytics"
}

variable "dataset" {
  description = "Target BigQuery dataset name."
  type        = string
  default     = "supercargo_github"
}

variable "vault_url" {
  description = "Supercargo Vault service URL for PII pseudonymization and decryption."
  type        = string
  default     = ""
}

variable "vault_audience" {
  description = "Supercargo Vault OIDC audience for authentication."
  type        = string
  default     = ""
}

variable "github_token_secret_id" {
  description = "Secret Manager secret ID containing the GitHub personal access token."
  type        = string
}

variable "service_account_id" {
  description = "Custom account ID for the Cloud Run Job service account."
  type        = string
  default     = "analytics-github-sync-sa"
}

variable "scheduler_service_account_id" {
  description = "Custom account ID for the Cloud Scheduler service account."
  type        = string
  default     = "analytics-github-sync-sched-sa"
}

variable "schedule" {
  description = "Cron schedule expression for Cloud Scheduler execution."
  type        = string
  default     = "0 * * * *"
}

variable "time_zone" {
  description = "Time zone for the Cloud Scheduler job."
  type        = string
  default     = "Etc/UTC"
}

variable "scheduler_job_name" {
  description = "Name of the Cloud Scheduler job. Defaults to ${var.job_name}-schedule if empty."
  type        = string
  default     = null
}

variable "scheduler_attempt_deadline" {
  description = "The deadline for Cloud Scheduler job attempts."
  type        = string
  default     = "320s"
}

variable "deletion_protection" {
  description = "Whether to protect the Cloud Run Job from deletion."
  type        = bool
  default     = false
}

variable "cpu_limit" {
  description = "CPU limit for the Cloud Run container."
  type        = string
  default     = "1"
}

variable "memory_limit" {
  description = "Memory limit for the Cloud Run container."
  type        = string
  default     = "1Gi"
}

variable "timeout" {
  description = "Execution timeout for the Cloud Run job."
  type        = string
  default     = "600s"
}

variable "max_retries" {
  description = "Number of retries for failed tasks."
  type        = number
  default     = 1
}
