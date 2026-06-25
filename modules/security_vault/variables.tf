variable "project_id" {
  type        = string
  description = "The GCP project ID where the security vault will be created."
}

variable "location" {
  type        = string
  description = "The GCP location for the dataset."
  default     = "US"
}

variable "env" {
  type        = string
  description = "The environment name."
}

variable "authorized_view_sa_email" {
  type        = string
  description = "The email of the service account authorized to view the data."
  default     = ""
}

variable "sovereign_kms_key_id" {
  type        = string
  description = "The fully qualified ID of the KMS key used for sovereign encryption."
}

variable "platform_topic_id" {
  type        = string
  description = "The Pub/Sub topic ID for the platform events (raw-platform)."
}
