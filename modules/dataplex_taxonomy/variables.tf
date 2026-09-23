variable "project_id" {
  description = "The Google Cloud Project ID where the Data Catalog taxonomy and policy tags will be created."
  type        = string
}

variable "region" {
  description = "The Google Cloud region/location for the taxonomy (e.g. eu, us, europe-west1)."
  type        = string
  default     = "eu"
}

variable "taxonomy_display_name" {
  description = "Display name for the Data Catalog sensitivity taxonomy."
  type        = string
  default     = "Corporate Sensitivity"
}

variable "taxonomy_description" {
  description = "Description for the Data Catalog sensitivity taxonomy."
  type        = string
  default     = "Enterprise sensitivity classification taxonomy for Supercargo data mesh governance"
}

variable "restricted_categories" {
  description = "Map of customer-defined sub-categories under the RESTRICTED sensitivity tier (e.g. { FINANCIAL = \"Financial records\", PERSONAL = \"Personal data\" })."
  type        = map(string)
  default = {
    FINANCIAL = "Financial and banking records requiring segregation of duties"
    PERSONAL  = "Personal and customer identifiers governed by GDPR / privacy regulations"
    SECURITY  = "Security tokens, credentials, and cryptographic material"
  }

  validation {
    condition     = alltrue([for k in keys(var.restricted_categories) : can(regex("^[a-zA-Z0-9_]+$", k))])
    error_message = "All keys in restricted_categories must contain only alphanumeric characters and underscores (e.g. FINANCIAL, PERSONAL, PHI)."
  }
}

