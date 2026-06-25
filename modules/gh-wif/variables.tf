variable "project_id" {
  description = "The GCP Project ID where the WIF pool will be created"
  type        = string
}

variable "pool_id" {
  description = "The ID of the Workload Identity Pool"
  type        = string
  default     = "github-pool"
}

variable "pool_display_name" {
  description = "Display name for the pool"
  type        = string
  default     = "GitHub Workload Identity Pool"
}

variable "provider_id" {
  description = "The ID of the Workload Identity Pool Provider"
  type        = string
  default     = "github-provider"
}

variable "repository" {
  description = "The GitHub repository in 'owner/repo' format"
  type        = string
}

variable "service_account_id" {
  description = "The account ID for the service account"
  type        = string
  default     = "github-actions-wif"
}
