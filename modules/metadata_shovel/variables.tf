variable "project_id" {
  description = "GCP Project ID"
  type        = string
}

variable "region" {
  description = "GCP Region"
  type        = string
}

variable "service_name" {
  description = "Cloud Run Service Name"
  type        = string
  default     = "metadata-shovel-internal"
}

variable "image" {
  description = "Container Image URL"
  type        = string
}

variable "firestore_database" {
  description = "Firestore Database ID"
  type        = string
  default     = "(default)"
}

variable "default_topic_id" {
  description = "Default Pub/Sub Topic ID (resource id)"
  type        = string
}

variable "topic_routing" {
  description = "Map of URN Prefix to Pub/Sub Topic ID"
  type        = map(string)
  default     = {}
}

variable "ingress_type" {
  description = "The type of ingress traffic allowed (INGRESS_TRAFFIC_ALL or INGRESS_TRAFFIC_INTERNAL_ONLY or INGRESS_TRAFFIC_INTERNAL_AND_GCLB)"
  type        = string
  default     = "INGRESS_TRAFFIC_INTERNAL_ONLY"
}
