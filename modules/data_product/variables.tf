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

