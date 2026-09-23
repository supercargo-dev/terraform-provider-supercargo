terraform {
  required_version = ">= 1.5.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 5.0.0"
    }
  }
}

provider "google" {
  project = "my-gov-project"
  region  = "europe-west1"
}

module "corporate_taxonomy" {
  source = "../../modules/dataplex_taxonomy"

  project_id            = "my-gov-project"
  region                = "europe-west1"
  taxonomy_display_name = "Corporate Sensitivity"
  taxonomy_description  = "Enterprise data sensitivity classification taxonomy"

  restricted_categories = {
    FINANCIAL = "Banking, credit card, and transactional records"
    PERSONAL  = "Direct customer identifiers and PII"
    SECURITY  = "Authentication credentials, private keys, and secrets"
    HEALTH    = "Protected health information (PHI)"
  }
}

output "taxonomy_id" {
  value = module.corporate_taxonomy.taxonomy_id
}

output "policy_tags" {
  value = module.corporate_taxonomy.policy_tags
}
