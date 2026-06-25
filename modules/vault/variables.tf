variable "project_id" {
  type        = string
  description = "The GCP project ID"
}

variable "project_number" {
  description = "The GCP Project Number"
  type        = string
}

variable "region" {
  type        = string
  description = "The GCP region"
  default     = "europe-west1"
}

variable "service_name" {
  type        = string
  description = "The name of the Cloud Run service"
  default     = "vault"
}

variable "image" {
  type        = string
  description = "The container image to deploy"
}

variable "ingress_type" {
  type        = string
  description = "The ingress type for the Cloud Run service"
  default     = "INGRESS_TRAFFIC_INTERNAL_ONLY"
}

variable "gateway_service_account_email" {
  type        = string
  description = "The service account email of the Ingestion Gateway"
}

variable "oidc_audience" {
  description = "The expected audience for OIDC tokens"
  type        = string
  default     = ""
}

variable "iap_enabled" {
  description = "Whether to enable IAP for the service"
  type        = bool
  default     = false
}

variable "iap_client_id" {
  description = "The OAuth 2.0 Client ID for IAP (manually created)"
  type        = string
  default     = ""
}

variable "iap_allowed_members" {
  description = "List of members to grant IAP access to"
  type        = list(string)
  default     = []
}

variable "authorized_invokers" {
  description = "List of IAM members authorized to invoke the vault directly"
  type        = list(string)
  default     = []
}

variable "auth_enforce" {
  description = "Whether to enforce OIDC authentication (Shadow Mode if false)"
  type        = bool
  default     = true
}

variable "custom_audiences" {
  description = "List of custom audiences to accept for OIDC"
  type        = list(string)
  default     = []
}

variable "force_deploy_trigger" {
  description = "A value that, when changed, triggers a new Cloud Run revision (e.g. git SHA or image digest). Use 'latest' or a fixed value for idempotency."
  type        = string
  default     = "static"
}
