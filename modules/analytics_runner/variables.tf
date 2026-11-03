variable "project_id" {
  description = "The Google Cloud project ID where resources will be provisioned."
  type        = string
}

variable "region" {
  description = "The Google Cloud region for the Cloud Run Job and Cloud Scheduler."
  type        = string
}

variable "job_name" {
  description = "Name of the Cloud Run v2 Job."
  type        = string
  default     = "analytics-runner"
}

variable "container_image" {
  description = "Container image URI for the analytics runner job."
  type        = string
}

variable "service_account_id" {
  description = "Custom account ID for the Cloud Run Job service account. Defaults to <job_name>-sa if null or empty."
  type        = string
  default     = null
}

variable "scheduler_service_account_id" {
  description = "Custom account ID for the Cloud Scheduler service account. Defaults to <job_name>-sched-sa if null or empty."
  type        = string
  default     = null
}

variable "schedule" {
  description = "Cron schedule expression for Cloud Scheduler execution."
  type        = string
  default     = "0 6 * * *"
}

variable "time_zone" {
  description = "Time zone for the Cloud Scheduler job."
  type        = string
  default     = "Etc/UTC"
}

variable "engine" {
  description = "Transformation engine to execute (e.g. dbt, sqlmesh)."
  type        = string
  default     = "dbt"
}

variable "dialect" {
  description = "SQL dialect / target warehouse (e.g. bigquery, snowflake, duckdb)."
  type        = string
  default     = "bigquery"
}

variable "target_datasets" {
  description = "List of BigQuery dataset IDs where runner has roles/bigquery.dataEditor permission."
  type        = list(string)
  default     = []
}

variable "input_datasets" {
  description = "List of BigQuery dataset IDs where runner has roles/bigquery.dataViewer permission."
  type        = list(string)
  default     = []
}

variable "cpu_limit" {
  description = "CPU limit for the Cloud Run container."
  type        = string
  default     = "2"
}

variable "memory_limit" {
  description = "Memory limit for the Cloud Run container."
  type        = string
  default     = "4Gi"
}

variable "timeout" {
  description = "Execution timeout for the Cloud Run job."
  type        = string
  default     = "1800s"
}

variable "max_retries" {
  description = "Number of retries for failed tasks."
  type        = number
  default     = 1
}

variable "paused" {
  description = "Whether the Cloud Scheduler job is paused."
  type        = bool
  default     = false
}

variable "deletion_protection" {
  description = "Whether to protect the Cloud Run Job from deletion."
  type        = bool
  default     = false
}
