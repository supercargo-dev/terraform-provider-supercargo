# Terraform Modules (`modules/`)

**Intent:** Reusable, parameterized Terraform modules for provisioning Supercargo services and infrastructure components.

## Key Capabilities
- **Standardization:** Encapsulates best practices (e.g., IAM, Cloud Run configs, h2c enablement) into reusable blocks.
- **Service Provisioning:** Dedicated modules for each core microservice, ensuring consistent deployments across environments.

## Directory Map
- `gateway/`: Provisions the Gateway service (Enforcement Plane).
- `hub/`: Provisions the Hub service (Control Plane).
- `loader/`: Provisions the Loader service (Batch Ingress).
- `metadata_shovel/`: Provisions the Event-driven metadata relay.
- `security_vault/`: Base security infrastructure for keys and secrets.
- `vault/`: Provisions the Vault service (Security Plane).
- `gh-wif/`: Provisions Workload Identity Federation for GitHub Actions.
- `dataplex_taxonomy/`: Provisions enterprise sensitivity taxonomies and policy tags in Google Cloud Data Catalog / Dataplex.
- `global_telemetry_share/`: Provisions customer-side BigQuery sharing dataset, IAM reader grants, and Authorized View access for federated platform telemetry.
- `analytics_github_sync/`: Provisions Cloud Run v2 Job and Cloud Scheduler trigger for autonomous topic-based GitHub ingestion into BigQuery.
- `docs_portal/`: Provisions Cloud Run v2 documentation portal fronted by GCLB with Identity-Aware Proxy (IAP) and HTTP-to-HTTPS redirect.
- `data_product/`: Provisions an end-to-end Data Product with declarative Pub/Sub streaming and DLT batch ingestion.
- `analytics_runner/`: Provisions Cloud Run v2 Job and Cloud Scheduler trigger for autonomous transformation pipelines (dbt, SQLMesh) in BigQuery.

## BC-7 Breaking Change & Migration Guide

Starting with Release BC-7, the Supercargo Terraform modules and provider standardize authentication, schema contract models, and IAM scoping to align with Google Cloud security best practices and eliminate plan-time cycle/dependency issues:

### 1. Backend Service-to-Service IAP Decommissioning (OIDC Audience Standard)
- **Architectural Rationale**: Google Cloud Run backend services do not support Identity-Aware Proxy (IAP) for service-to-service gRPC or HTTP communication. Authenticated inter-service communication uses native Google Cloud IAM OIDC tokens with audience verification.
- **Breaking Change**: The legacy IAP variables (`iap_enabled`, `iap_client_id`, `hub_iap_client_id`, `vault_iap_client_id`) have been removed from `hub`, `vault`, `gateway`, and `data_product` modules.
- **Migration**: Replace IAP client ID inputs with OIDC audience variables (`hub_oidc_audience`, `vault_oidc_audience`). By default, these default to their respective service URLs (`var.hub_address`, `var.kms_address`).

#### Before (Legacy):
```hcl
module "data_product" {
  source = "github.com/supercargo-dev/terraform-provider-supercargo//modules/data_product"

  project_id          = "my-gcp-project"
  region              = "europe-west1"
  hub_address         = "https://hub.internal.supercargo.net"
  kms_address         = "https://vault.internal.supercargo.net"
  hub_iap_client_id   = "123456789-abc.apps.googleusercontent.com"
  vault_iap_client_id = "123456789-xyz.apps.googleusercontent.com"
}
```

#### After (BC-7):
```hcl
module "data_product" {
  source = "github.com/supercargo-dev/terraform-provider-supercargo//modules/data_product"

  project_id          = "my-gcp-project"
  region              = "europe-west1"
  hub_address         = "https://hub.internal.supercargo.net"
  kms_address         = "https://vault.internal.supercargo.net"
  hub_oidc_audience   = "https://hub.internal.supercargo.net"
  vault_oidc_audience = "https://vault.internal.supercargo.net"
}
```

### 2. Standardization of `contracts.*.schema_json` (Removal of `schema`)
- **Architectural Rationale**: Contract schemas are represented as canonical BigQuery JSON schema strings. Standardizing on `schema_json` across the HCL contract map, the `supercargo_data_product` data source, and `modules/data_product` eliminates confusion with nested schema objects.
- **Breaking Change**: The deprecated `contracts.*.schema` attribute has been removed. Contract objects in `modules/data_product` and outputs from `data.supercargo_data_product.this.contracts` expose `schema_json`.
- **Migration**: In module calls and resource definitions, rename references from `schema` to `schema_json`.

#### Before (Legacy):
```hcl
resource "google_bigquery_table" "tables" {
  for_each   = module.data_product.contracts
  dataset_id = google_bigquery_dataset.this.dataset_id
  table_id   = each.key
  schema     = each.value.schema
}
```

#### After (BC-7):
```hcl
resource "google_bigquery_table" "tables" {
  for_each   = module.data_product.contracts
  dataset_id = google_bigquery_dataset.this.dataset_id
  table_id   = each.key
  schema     = each.value.schema_json
}
```

### 3. Least-Privilege Dataset-Scoped BigQuery IAM in `analytics_github_sync`
- **Architectural Rationale**: Granting project-level `roles/bigquery.dataEditor` violates the principle of least privilege by allowing write permissions across all BigQuery datasets in the GCP project.
- **Change**: `roles/bigquery.dataEditor` is now strictly scoped to the target dataset via `google_bigquery_dataset_iam_member.sync_job_bq_editor`. Project-level permissions are confined to `roles/bigquery.jobUser` via `google_project_iam_member.sync_job_bq_user` to permit query job execution.
- **Migration**: Ensure the target BigQuery dataset (`var.dataset`) exists before provisioning or applying the `analytics_github_sync` module.

### 4. Plan-Time Deterministic Service Account Formulas
- **Architectural Rationale**: Passing dynamic resource attributes (`google_service_account.<name>.email`) directly into `for_each` or `count` expressions causes Terraform evaluation crashes during `terraform plan` because values are `(known after apply)`.
- **Standard Pattern**: All modules derive service account emails statically via `locals` using the deterministic string formula:
  ```hcl
  locals {
    sa_email = "${var.service_account_id != "" ? var.service_account_id : "default-sa-name"}@${var.project_id}.iam.gserviceaccount.com"
  }
  ```
- **Recommendation**: Module consumers and cross-project IAM configurations should reference deterministic service account formulas (`<sa-name>@${var.project_id}.iam.gserviceaccount.com`) rather than dynamic resource outputs when configuring plan-sensitive collections.
