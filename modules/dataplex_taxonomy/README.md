# Google Cloud Dataplex / Data Catalog Taxonomy Module

Reusable Terraform module for provisioning enterprise data sensitivity taxonomies and policy tags in Google Cloud Data Catalog / Dataplex for Supercargo data mesh governance.

## Features

- **Standard Base Sensitivity Tiers**: Deterministically provisions standard tiers (`PUBLIC`, `INTERNAL`, `CONFIDENTIAL`, `RESTRICTED`) with Fine-Grained Access Control enabled.
- **Customer Parameterized Sub-Categories**: Allows organizations to formalize domain-specific qualified categories (e.g. `RESTRICTED:FINANCIAL`, `RESTRICTED:PERSONAL`, `RESTRICTED:SECURITY`, `RESTRICTED:HEALTH`) under the `RESTRICTED` parent tag.
- **Plan-Time Determinism**: Constructed with static local keys to ensure 100% deterministic Terraform plans without relying on unapplied attributes in `for_each`.
- **Zero-Config Compatibility**: Emits canonical policy tag URN maps (`outputs.policy_tags`) directly consumable by Supercargo Hub and BigQuery physical configurations.

## Usage

```hcl
module "corporate_taxonomy" {
  source = "github.com/supercargo-dev/terraform-provider-supercargo//modules/dataplex_taxonomy"

  project_id            = "my-governance-project"
  region                = "eu"
  taxonomy_display_name = "Corporate Sensitivity"
  taxonomy_description  = "Enterprise sensitivity classification taxonomy for Supercargo data mesh"

  restricted_categories = {
    FINANCIAL = "Banking, credit card, and transactional records requiring segregation of duties"
    PERSONAL  = "Personal and customer identifiers governed by GDPR / privacy regulations"
    SECURITY  = "Security tokens, credentials, and cryptographic material"
    HEALTH    = "Protected health information (PHI)"
  }
}

output "policy_tag_map" {
  value = module.corporate_taxonomy.policy_tags
}
```

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| `project_id` | Google Cloud Project ID where taxonomy is created | `string` | n/a | yes |
| `region` | Google Cloud region/location (e.g. `eu`, `us`, `europe-west1`) | `string` | `"eu"` | no |
| `taxonomy_display_name` | Display name for the taxonomy | `string` | `"Corporate Sensitivity"` | no |
| `taxonomy_description` | Description for the taxonomy | `string` | `"Enterprise sensitivity classification taxonomy for Supercargo data mesh governance"` | no |
| `restricted_categories` | Map of customer-defined sub-categories under `RESTRICTED` | `map(string)` | `{ FINANCIAL = ..., PERSONAL = ..., SECURITY = ... }` | no |

## Outputs

| Name | Description |
|------|-------------|
| `taxonomy_id` | Unique identifier of the created Google Data Catalog taxonomy |
| `taxonomy_name` | Resource name of the taxonomy (`projects/{project}/locations/{location}/taxonomies/{taxonomy}`) |
| `taxonomy_urn` | Canonical URN for Dataplex / Data Catalog integration |
| `base_policy_tags` | Map of base sensitivity tiers to policy tag IDs |
| `restricted_policy_tags` | Map of qualified sub-categories (`RESTRICTED:<CATEGORY>`) to policy tag IDs |
| `policy_tags` | Unified map of all tiers and qualified categories to policy tag IDs |
