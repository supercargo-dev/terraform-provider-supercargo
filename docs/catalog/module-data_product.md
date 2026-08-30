---
type: Supercargo Terraform Module
title: data_product Module
description: Terraform module for deploying Data Products and their Data Contracts into the Supercargo Hub.
tags: [terraform, module, okf, iam, security]
---

# data_product Module

Terraform module for deploying Data Products and their Data Contracts into the Supercargo Hub, configuring the associated ingestion Gateway, Pub/Sub event topics, BigQuery streaming sinks, and IAM invocation controls.

## Golden Path Usage
```terraform
module "customer_telemetry" {
  source         = "github.com/supercargo-dev/terraform-provider-supercargo//modules/data_product"
  project_id     = var.project_id
  region         = var.region
  manifest_file  = "${path.module}/product.yaml"
  image          = "europe-west1-docker.pkg.dev/supercargo-dev/images/gateway:latest"
  hub_address    = "https://hub-ezwd7z77ha-ew.a.run.app"
  kms_address    = "https://vault-ezwd7z77ha-ew.a.run.app"
  master_key_uri = "projects/supercargo-dev/locations/europe-west1/keyRings/supercargo-vault-keyring/cryptoKeys/supercargo-vault-master-key"

  # Declarative authorization of application service accounts
  authorized_invoker_service_accounts = [
    "checkout-service@supercargo-prod.iam.gserviceaccount.com",
    "payment-service@supercargo-prod.iam.gserviceaccount.com"
  ]

  # Additional IAM members (users, groups, service accounts with prefix)
  authorized_invokers = [
    "group:data-platform-admins@example.com"
  ]
}
```

## Key Capabilities
- **Native Manifest Contract Resolution**: Auto-resolves contract files, schema JSONs, and content hashes from `product.yaml`.
- **Dynamic Table Wiring**: Dynamically supplies BigQuery table configurations to the underlying `gateway` module with normalized table identifiers.
- **Declarative IAM Invocation Controls**: Exposes `authorized_invoker_service_accounts` (convenient raw service account emails automatically formatted as `serviceAccount:...`) and `authorized_invokers` (arbitrary IAM member expressions) to grant `roles/run.invoker` on the deployed Cloud Run Gateway service.
- **Encapsulated Pub/Sub Push Invocation**: Automatically creates and binds a dedicated push invoker service account for Pub/Sub push subscription delivery to Cloud Run.
- **Backwards Compatibility**: Supports explicit contract maps via `var.contracts` for CI/CD artifact-passing pipelines.

## Configuration Variables
| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `manifest_file` | `string` | *required* | Path to the Data Product manifest YAML/JSON file. |
| `project_id` | `string` | *required* | The GCP project ID where resources are deployed. |
| `region` | `string` | *required* | The GCP region for Cloud Run and Pub/Sub resources. |
| `image` | `string` | *required* | The container image for the Gateway service. |
| `hub_address` | `string` | *required* | The address/URL of the Hub service. |
| `kms_address` | `string` | *required* | The address/URL of the KMS (Vault) service. |
| `master_key_uri` | `string` | *required* | The URI of the Master Key in GCP KMS for envelope encryption. |
| `authorized_invoker_service_accounts` | `list(string)` | `[]` | List of service account emails authorized to invoke the Gateway service (auto-prefixed with `serviceAccount:`). |
| `authorized_invokers` | `list(string)` | `[]` | List of raw IAM members (`user:...`, `group:...`, `serviceAccount:...`) authorized to invoke the Gateway. |
| `contracts` | `map(object(...))` | `{}` | Optional explicit map of contract configurations to register. |
| `bigquery_dataset_id` | `string` | `""` | Optional BigQuery dataset ID for validated data sink tables. |
| `auth_enforce` | `bool` | `true` | Whether to enforce OIDC token authentication at runtime (Shadow Mode if false). |
| `bigquery_deletion_protection` | `bool` | `true` | Whether to enable deletion protection on generated BigQuery tables. |

## Module Outputs
| Output | Type | Description |
| :--- | :--- | :--- |
| `product_urn` | `string` | The URN of the registered Data Product in Supercargo Hub. |
| `gateway_service_name` | `string` | The name of the deployed Gateway Cloud Run service. |
| `gateway_url` | `string` | The HTTPS URL of the deployed Cloud Run Gateway. |
| `raw_topic_id` | `string` | The ID of the Pub/Sub topic for raw unvalidated events. |
| `clean_topic_id` | `string` | The ID of the Pub/Sub topic for validated clean events. |
| `dlq_topic_id` | `string` | The ID of the Dead Letter Queue (DLQ) Pub/Sub topic for quarantine routing. |
| `push_invoker_service_account_email` | `string` | The email of the dedicated Pub/Sub Push Invoker Service Account. |
| `raw_subscription_id` | `string` | The ID of the raw event Pub/Sub push subscription triggering the Gateway. |
| `dlq_subscription_id` | `string` | The ID of the Dead Letter Queue Pub/Sub subscription. |
| `service_account_email` | `string` | The email of the runtime Gateway Service Account. |
| `bigquery_table_ids` | `map(string)` | Map of contract IDs to their destination BigQuery table IDs. |
| `contracts` | `map(object)` | Map of auto-discovered or explicitly provided contracts. |

## Implementation
- Source Code: [modules/data_product](../../modules/data_product)
