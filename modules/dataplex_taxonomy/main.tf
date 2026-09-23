locals {
  base_tiers = {
    PUBLIC       = "Public data, accessible without restriction across all domains."
    INTERNAL     = "Internal data, default access tier for enterprise workforce."
    CONFIDENTIAL = "Confidential business data requiring explicit business justification."
    RESTRICTED   = "Restricted sensitive data requiring strict access controls, pseudonymization, and audit trails."
  }

  # Normalize customer restricted categories into uppercase keys for deterministic matching
  restricted_categories = {
    for k, v in var.restricted_categories : upper(trimspace(k)) => v
  }
}

resource "google_data_catalog_taxonomy" "sensitivity" {
  project                = var.project_id
  region                 = var.region
  display_name           = var.taxonomy_display_name
  description            = var.taxonomy_description
  activated_policy_types = ["FINE_GRAINED_ACCESS_CONTROL"]
}

resource "google_data_catalog_policy_tag" "base_tiers" {
  for_each     = local.base_tiers
  taxonomy     = google_data_catalog_taxonomy.sensitivity.id
  display_name = each.key
  description  = each.value
}

resource "google_data_catalog_policy_tag" "restricted_subcategories" {
  for_each          = local.restricted_categories
  taxonomy          = google_data_catalog_taxonomy.sensitivity.id
  display_name      = each.key
  description       = each.value
  parent_policy_tag = google_data_catalog_policy_tag.base_tiers["RESTRICTED"].id
}
