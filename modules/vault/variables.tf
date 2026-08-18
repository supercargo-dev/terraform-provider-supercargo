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
  validation {
    condition = contains([
      "INGRESS_TRAFFIC_ALL",
      "INGRESS_TRAFFIC_INTERNAL_ONLY",
      "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"
    ], var.ingress_type)
    error_message = "ingress_type must be one of: INGRESS_TRAFFIC_ALL, INGRESS_TRAFFIC_INTERNAL_ONLY, INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER."
  }
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
  validation {
    condition = alltrue([
      for m in var.authorized_invokers : !contains(["allUsers", "allAuthenticatedUsers"], m)
    ])
    error_message = "authorized_invokers must not contain 'allUsers' or 'allAuthenticatedUsers'."
  }
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

variable "keyring_name" {
  description = "The name of the Cloud KMS KeyRing for the Vault. Defaults to 'supercargo-vault-keyring' (stable naming without random suffixes to prevent recreation of immutable KMS resources)."
  type        = string
  default     = "supercargo-vault-keyring"
  validation {
    condition     = can(regex("^[a-zA-Z0-9_-]{1,63}$", var.keyring_name))
    error_message = "keyring_name must be 1-63 characters containing only letters, numbers, underscores, and hyphens."
  }
}

variable "crypto_key_name" {
  description = "The name of the Cloud KMS CryptoKey for the Vault master key. Defaults to 'supercargo-vault-master-key'."
  type        = string
  default     = "supercargo-vault-master-key"
  validation {
    condition     = can(regex("^[a-zA-Z0-9_-]{1,63}$", var.crypto_key_name))
    error_message = "crypto_key_name must be 1-63 characters containing only letters, numbers, underscores, and hyphens."
  }
}

variable "key_rotation_period" {
  description = "The rotation period for the Cloud KMS master key (e.g. '2592000s' for 30 days)."
  type        = string
  default     = "2592000s"
  validation {
    condition     = can(regex("^[1-9][0-9]*s$", var.key_rotation_period))
    error_message = "key_rotation_period must be a positive integer followed by 's' (e.g. '2592000s')."
  }
}
