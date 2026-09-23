output "taxonomy_id" {
  description = "The unique identifier of the created Google Data Catalog taxonomy."
  value       = google_data_catalog_taxonomy.sensitivity.id
}

output "taxonomy_name" {
  description = "The resource name of the created Google Data Catalog taxonomy (projects/{project}/locations/{location}/taxonomies/{taxonomy})."
  value       = google_data_catalog_taxonomy.sensitivity.name
}

output "taxonomy_urn" {
  description = "The canonical URN of the created taxonomy in Dataplex / Data Catalog format."
  value       = google_data_catalog_taxonomy.sensitivity.id
}

output "base_policy_tags" {
  description = "Map of base sensitivity tiers (PUBLIC, INTERNAL, CONFIDENTIAL, RESTRICTED) to their policy tag resource IDs."
  value = {
    for k, v in google_data_catalog_policy_tag.base_tiers : k => v.id
  }
}

output "restricted_policy_tags" {
  description = "Map of customer-defined sub-categories (e.g. RESTRICTED:FINANCIAL) to their policy tag resource IDs."
  value = {
    for k, v in google_data_catalog_policy_tag.restricted_subcategories : "RESTRICTED:${k}" => v.id
  }
}

output "policy_tags" {
  description = "Unified map of all sensitivity tiers and qualified sub-categories to their policy tag resource IDs (compatible with Supercargo Hub and BigQuery physical config)."
  value = merge(
    { for k, v in google_data_catalog_policy_tag.base_tiers : k => v.id },
    { for k, v in google_data_catalog_policy_tag.restricted_subcategories : "RESTRICTED:${k}" => v.id }
  )
}
