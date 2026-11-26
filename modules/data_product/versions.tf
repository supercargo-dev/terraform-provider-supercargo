terraform {
  required_providers {
    supercargo = {
      source  = "supercargo-dev/supercargo"
      version = ">= 0.1.0"
    }
    google = {
      source  = "hashicorp/google"
      version = ">= 6.0.0"
    }
  }
}
