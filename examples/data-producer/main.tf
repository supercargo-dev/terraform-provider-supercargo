terraform {
  required_providers {
    supercargo = {
      source  = "supercargo-dev/supercargo"
      version = ">= 0.1.0"
    }
    google = {
      source  = "hashicorp/google"
      version = "~> 7.0"
    }
  }
}

provider "supercargo" {
  hub_address = "api.supercargo.example.com:443"
}

provider "google" {
  project = "my-gcp-project"
  region  = "europe-west1"
}

module "customer_telemetry" {
  source = "../../modules/data_product"

  # 1. Logical configuration (Hub) - Contracts are auto-discovered from manifest_file
  manifest_file = "${path.module}/dataproduct.yaml"

  # 2. Physical infrastructure (GCP Data Plane)
  project_id          = "my-gcp-project"
  region              = "europe-west1"
  bigquery_dataset_id = "customer_telemetry"

  # Gateway container configuration
  image          = "gcr.io/supercargo/gateway:latest"
  hub_address    = "api.supercargo.example.com:443"
  kms_address    = "kms.supercargo.example.com:443"
  master_key_uri = "gcp-kms://projects/..."

  # Ensure the gateway enforces authentication
  auth_enforce = true
}

output "gateway_url" {
  description = "The URL of the deployed Cloud Run Gateway"
  value       = module.customer_telemetry.gateway_url
}

output "product_urn" {
  description = "The Supercargo URN of the registered product"
  value       = module.customer_telemetry.product_urn
}
