# Global Telemetry Share Terraform Module (`modules/global_telemetry_share`)

Reusable Terraform module that provisions the customer-side BigQuery sharing dataset, IAM reader grants, Authorized View dataset access grants back to `supercargo_intelligence_internal` and `supercargo_github`, and optional BigQuery Analytics Hub listing for federated platform telemetry.

## Architecture & Security Boundary

```
+-------------------------------------------------------------------------------+
| Customer GCP Project (var.project_id)                                         |
|                                                                               |
|   +------------------------------------+   +------------------------------+   |
|   | Dataset:                           |   | Dataset:                     |   |
|   | supercargo_intelligence_internal   |   | supercargo_github            |   |
|   |   [internal rollup tables]         |   |   [pull_requests]            |   |
|   +-----------------+------------------+   +--------------+---------------+   |
|                     ^                                     ^                   |
|                     | Authorized View Grants              |                   |
|                     | (google_bigquery_dataset_access)    |                   |
|                     +------------------+------------------+                   |
|                                        |                                      |
|   +------------------------------------+                                      |
|   | Shared Dataset:                                                           |
|   | supercargo_global_telemetry                                               |
|   |                                                                           |
|   |   [View: global_telemetry_digest]  |<------- Read-Only (dataViewer)       |
|   +------------------------------------+         (supercargo_reader_principal)|
+-------------------------------------------------------------------------------+
```

### Security Invariants
1. **Authorized View Boundary:** The central Supercargo telemetry reader principal (`supercargo_reader_principal`) is granted `roles/bigquery.dataViewer` **only** on the shared dataset (`var.dataset_id`, defaulting to `supercargo_global_telemetry`).
2. **Internal Dataset Isolation:** The reader principal has **no direct access** to `supercargo_intelligence_internal` or `supercargo_github`. Query execution on `global_telemetry_digest` accesses underlying tables strictly through BigQuery's Authorized View delegation mechanism.
3. **Sovereignty & Revocation:** Customers can disable sharing instantly at any time by setting `enable_global_telemetry_share = false`, which destroys the shared dataset (`delete_contents_on_destroy = true`), IAM access, and Authorized View grants without impacting local analytics pipelines.

## Provisioning Sequence
In Google Cloud BigQuery, an Authorized View grant validates that the target view exists. Recommended order of operations:
1. Run Dataform to compile and materialize models: `npx @dataform/cli run`.
2. Apply Terraform to create dataset permissions and Authorized View delegations.

## Usage

```hcl
module "global_telemetry_share" {
  source = "../../modules/global_telemetry_share"

  project_id                    = "my-customer-gcp-project"
  dataset_id                    = "supercargo_global_telemetry"
  internal_dataset_id           = "supercargo_intelligence_internal"
  github_dataset_id             = "supercargo_github"
  enable_global_telemetry_share = true
  location                      = "EU"

  # Optional BigQuery Analytics Hub Listing
  enable_analytics_hub_listing  = false
}
```

## Requirements

| Name | Version |
|------|---------|
| terraform | >= 1.5.0 |
| google | >= 5.0.0, < 8.0.0 |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| `project_id` | GCP project ID where the shared BigQuery dataset will be created | `string` | n/a | yes |
| `dataset_id` | ID of the shared BigQuery dataset | `string` | `"supercargo_global_telemetry"` | no |
| `internal_dataset_id` | ID of internal dataset containing source tables for the authorized view | `string` | `"supercargo_intelligence_internal"` | no |
| `github_dataset_id` | ID of GitHub dataset containing pull requests and issues | `string` | `"supercargo_github"` | no |
| `enable_global_telemetry_share` | Feature flag to enable or disable global telemetry sharing resources | `bool` | `true` | no |
| `supercargo_reader_principal` | IAM principal for central Supercargo telemetry reader service account | `string` | `"serviceAccount:telemetry-reader@supercargo-global.iam.gserviceaccount.com"` | no |
| `location` | GCP region/location for BigQuery resources | `string` | `"EU"` | no |
| `enable_analytics_hub_listing` | Whether to publish the shared dataset to BigQuery Analytics Hub | `bool` | `false` | no |
| `data_exchange_id` | ID of the Analytics Hub Data Exchange (required if `enable_analytics_hub_listing` is true) | `string` | `""` | no |
| `enable_global_sdlc_share` | Feature flag to enable or disable global SDLC efficiency sharing resources | `bool` | `true` | no |
| `enable_peer_benchmark_subscription` | Whether to subscribe to the fleet-wide anonymous peer benchmarks via Analytics Hub | `bool` | `false` | no |
| `peer_benchmark_data_exchange_id` | Analytics Hub Data Exchange ID for peer benchmarks (defaults to data_exchange_id if empty) | `string` | `""` | no |
| `peer_benchmark_listing_id` | Analytics Hub Listing ID for peer benchmarks (required if `enable_peer_benchmark_subscription` is true) | `string` | `""` | no |
| `peer_benchmark_dataset_id` | Destination dataset ID for reciprocal peer benchmark subscription | `string` | `"supercargo_peer_benchmarks"` | no |

## Outputs

| Name | Description |
|------|-------------|
| `dataset_id` | The ID of the shared BigQuery dataset |
| `dataset_access_id` | Resource ID of the reader dataset access grant |
| `authorized_view_access_id` | Resource ID of the Authorized View grant on the internal dataset |
| `authorized_view_github_access_id` | Resource ID of the Authorized View grant on the GitHub dataset |
| `authorized_view_sdlc_access_id` | Resource ID of the Authorized View grant for SDLC efficiency metrics on the internal dataset (`global_sdlc_efficiency_digest`) |
| `authorized_view_github_sdlc_access_id` | Resource ID of the Authorized View grant for SDLC efficiency metrics on the GitHub dataset (`global_sdlc_efficiency_digest`) |
| `analytics_hub_listing_id` | Resource ID of the Analytics Hub listing |
| `sdlc_analytics_hub_listing_id` | Resource ID of the Analytics Hub listing for SDLC efficiency metrics |
| `peer_benchmark_subscription_id` | Resource ID of the reciprocal peer benchmark Analytics Hub subscription |
| `sharing_enabled` | Whether global telemetry sharing is active |
| `sdlc_sharing_enabled` | Whether global SDLC efficiency sharing is active |
| `peer_benchmarks_enabled` | Whether peer benchmark subscription is active |

