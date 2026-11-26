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

variable "name_suffix" {
  description = "Optional deterministic suffix appended to resource names for environment/branch disambiguation. Defaults to empty."
  type        = string
  default     = ""
}

variable "telemetry_identity_salt" {
  description = "Identity salt for vault operational telemetry hashing"
  type        = string
  default     = "supercargo-default-salt"
}

variable "telemetry_topic_id" {
  description = "Target Pub/Sub topic ID for vault operational telemetry"
  type        = string
  default     = "supercargo-platform-telemetry"
}

variable "authorized_tokenizer_sa" {
  description = "Authorized tokenizer service account email"
  type        = string
  default     = ""
}

variable "max_instances" {
  description = "The maximum number of instances for the Vault service"
  type        = number
  default     = 3
}

# --- Cloud Monitoring Variables ---

variable "enable_monitoring" {
  description = "Whether to enable Cloud Monitoring alerts for Vault"
  type        = bool
  default     = true
}

variable "alert_notification_channels" {
  description = "List of Cloud Monitoring notification channel IDs for alert routing"
  type        = list(string)
  default     = []
}

variable "sla_tier" {
  description = "The SLA tier (TIER_1, TIER_2, TIER_3)"
  type        = string
  default     = "TIER_1"
  validation {
    condition     = contains(["TIER_1", "TIER_2", "TIER_3"], var.sla_tier)
    error_message = "The sla_tier variable must be one of: TIER_1, TIER_2, TIER_3."
  }
}

# --- SearchHash PAM JIT Variables ---

variable "enable_search_hash_pam" {
  description = "Whether to provision a GCP PAM JIT entitlement for Vault SearchHash operations."
  type        = bool
  default     = false
}

variable "search_hash_pam_location" {
  description = "GCP PAM location ('global' or a regional identifier such as 'us-central1') for the Vault SearchHash entitlement."
  type        = string
  default     = "global"
  validation {
    condition     = can(regex("^(global|[a-z]+-[a-z]+[0-9]+)$", var.search_hash_pam_location))
    error_message = "search_hash_pam_location must be 'global' or a valid GCP region identifier matching ^(global|[a-z]+-[a-z]+[0-9]+)$."
  }
}

variable "search_hash_pam_max_duration" {
  description = "Maximum GCP PAM grant duration in seconds (bounded between 900s and 3600s) for Vault SearchHash."
  type        = string
  default     = "3600s"
  validation {
    condition     = can(regex("^[0-9]+s$", var.search_hash_pam_max_duration)) && tonumber(trimsuffix(var.search_hash_pam_max_duration, "s")) >= 900 && tonumber(trimsuffix(var.search_hash_pam_max_duration, "s")) <= 3600
    error_message = "search_hash_pam_max_duration must be an integer seconds string between 900s and 3600s (inclusive)."
  }
}

variable "search_hash_pam_requesters" {
  description = "List of IAM principals (user:, group:, serviceAccount:, domain:) eligible to request PAM JIT access for Vault SearchHash."
  type        = list(string)
  default     = []
  validation {
    condition = alltrue([
      for m in var.search_hash_pam_requesters : can(regex("^(user|group|serviceAccount|domain):.+$", trimspace(m)))
    ])
    error_message = "Every entry in search_hash_pam_requesters must start with user:, group:, serviceAccount:, or domain:."
  }
}

variable "search_hash_pam_approvers" {
  description = "List of IAM principals (user:, group:, serviceAccount:, domain:) authorized to approve Vault SearchHash PAM JIT requests."
  type        = list(string)
  default     = []
  validation {
    condition = alltrue([
      for m in var.search_hash_pam_approvers : can(regex("^(user|group|serviceAccount|domain):.+$", trimspace(m)))
    ])
    error_message = "Every entry in search_hash_pam_approvers must start with user:, group:, serviceAccount:, or domain:."
  }
}

variable "enforce_search_justification" {
  description = "Whether Vault SearchHash requires an explicit ticket_id justification on every request."
  type        = bool
  default     = true
}

variable "search_hash_allowed_roles" {
  description = "Optional list of Vault RBAC roles/principals permitted to call SearchHash (when empty, Vault defaults to privacyOfficer/complianceAuditor)."
  type        = list(string)
  default     = []
}


