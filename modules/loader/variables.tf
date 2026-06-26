variable "project_id" {
  description = "GCP Project ID"
  type        = string
}

variable "region" {
  description = "GCP Region"
  type        = string
  default     = "europe-west1"
}

variable "domain_name" {
  description = "Data Domain Name (e.g. finance, marketing)"
  type        = string
}

variable "image" {
  description = "Container Image URL"
  type        = string
}

variable "ingress_config" {
  description = "JSON Configuration for the Ingress Service"
  type        = string
}

variable "bucket_name" {
  description = "Source GCS Bucket Name"
  type        = string
}

variable "target_topic_ids" {
  description = "List of Target Pub/Sub Topic IDs to grant publish permissions on"
  type        = list(string)
}

variable "dlq_topic_id" {
  description = "Pub/Sub Topic ID for Dead Letter Queue"
  type        = string
}

variable "ingress_type" {
  description = "The type of ingress traffic allowed (INGRESS_TRAFFIC_ALL or INGRESS_TRAFFIC_INTERNAL_ONLY or INGRESS_TRAFFIC_INTERNAL_AND_GCLB)"
  type        = string
  default     = "INGRESS_TRAFFIC_ALL"
}
