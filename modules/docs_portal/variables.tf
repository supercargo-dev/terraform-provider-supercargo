variable "project_id" {
  description = "The GCP project ID where resources will be provisioned"
  type        = string
}

variable "region" {
  description = "The GCP region for regional resources (Cloud Run, Serverless NEG)"
  type        = string
  default     = "europe-west1"
}

variable "service_name" {
  description = "The name of the Cloud Run service and base naming prefix for load balancer resources"
  type        = string
  default     = "docs-portal"
}

variable "image" {
  description = "The container image to deploy to Cloud Run"
  type        = string
}

variable "domain_name" {
  description = "The fully qualified custom domain name for the docs portal (e.g., docs.example.com)"
  type        = string
}

variable "licensed_domains" {
  description = "List of corporate domains authorized to access the docs portal via IAP (e.g., ['example.com'] or ['domain:example.com'])"
  type        = list(string)
  default     = []
}

variable "iap_enabled" {
  description = "Whether to enable Identity-Aware Proxy (IAP) on the GCLB backend service"
  type        = bool
  default     = true
}

variable "iap_client_id" {
  description = "OAuth 2.0 Client ID for Identity-Aware Proxy (IAP)"
  type        = string
  default     = ""
}

variable "iap_client_secret" {
  description = "OAuth 2.0 Client Secret for Identity-Aware Proxy (IAP)"
  type        = string
  sensitive   = true
  default     = ""
}
