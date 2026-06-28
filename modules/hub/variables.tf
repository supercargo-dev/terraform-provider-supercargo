variable "project_id" {
  description = "The GCP Project ID"
  type        = string
}

variable "project_number" {
  description = "The GCP Project Number"
  type        = string
}

variable "region" {
  description = "The GCP region"
  type        = string
}

variable "hub_image_tag" {
  description = "The image tag for the hub service (Git SHA). Optional for local infra updates."
  type        = string
  default     = ""
}

variable "environment" {
  description = "The environment name (dev, staging, prod)"
  type        = string
}

variable "firestore_project_id" {
  description = "Project ID where Firestore resides"
  type        = string
}

variable "image_project_id" {
  description = "Project ID where the Artifact Registry resides"
  type        = string
}

variable "gateway_service_account_email" {
  description = "The service account email of the Ingestion Gateway"
  type        = string
  default     = null
}

variable "create_gateway_invoker" {
  description = "Whether to create the IAM binding for the Gateway service account"
  type        = bool
  default     = false
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

variable "oidc_audience" {
  description = "The expected audience for OIDC tokens"
  type        = string
  default     = ""
}

variable "shovel_image_tag" {
  description = "The image tag for the metadata_shovel service"
  type        = string
  default     = ""
}

variable "control_plane_topic" {
  description = "The Pub/Sub topic ID for the control plane ingestion"
  type        = string
}

variable "platform_trust_team" {
  description = "The name of the platform trust team (or email of trusted SA) for emergency bypass"
  type        = string
  default     = "platform-trust"
}

variable "ingress_type" {
  description = "The type of ingress traffic allowed (INGRESS_TRAFFIC_ALL or INGRESS_TRAFFIC_INTERNAL_ONLY or INGRESS_TRAFFIC_INTERNAL_AND_GCLB)"
  type        = string
  default     = "INGRESS_TRAFFIC_ALL"
}

variable "authorized_invokers" {
  description = "List of IAM members authorized to invoke the hub directly"
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

variable "iam_domain" {
  description = "The domain for IAM groups and users"
  type        = string
  default     = "example.com"
}

variable "topic_routing" {
  description = "A map of URN prefixes to Pub/Sub topic names for custom routing in the metadata shovel."
  type        = map(string)
  default     = {}
}
