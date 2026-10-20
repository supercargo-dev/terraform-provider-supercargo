variable "project_id" {
  description = "The GCP project ID where the shared BigQuery dataset will be created"
  type        = string
}

variable "dataset_id" {
  description = "The ID of the shared BigQuery dataset"
  type        = string
  default     = "supercargo_global_telemetry"
}

variable "internal_dataset_id" {
  description = "The ID of the internal intelligence dataset containing the authorized view source tables"
  type        = string
  default     = "supercargo_intelligence_internal"
}

variable "github_dataset_id" {
  description = "The ID of the GitHub dataset containing pull requests and issues"
  type        = string
  default     = "supercargo_github"
}

variable "enable_global_telemetry_share" {
  description = "Feature flag to enable or disable global telemetry sharing resources"
  type        = bool
  default     = true
}

variable "supercargo_reader_principal" {
  description = "IAM principal for the central Supercargo telemetry reader service account"
  type        = string
  default     = "serviceAccount:telemetry-reader@supercargo-global.iam.gserviceaccount.com"
}

variable "location" {
  description = "The GCP region/location for BigQuery resources"
  type        = string
  default     = "EU"
}

variable "enable_analytics_hub_listing" {
  description = "Whether to publish the shared dataset to BigQuery Analytics Hub"
  type        = bool
  default     = false
}

variable "data_exchange_id" {
  description = "The ID of the Analytics Hub Data Exchange (required if enable_analytics_hub_listing is true)"
  type        = string
  default     = ""
}
