---
type: Supercargo Terraform Module
title: gateway Module
description: Terraform module for deploying the Data Plane Gateway on Cloud Run, which enforces ingestion rules.
tags: [terraform, module, okf, iam, security]
---

# gateway Module

Terraform module for deploying the Data Plane Gateway on Cloud Run, which enforces ingestion rules, schema validation, cryptographic pseudonymization, and DLQ quarantine routing.

## Golden Path Usage
```terraform
module "gateway" {
  source         = "github.com/supercargo-dev/terraform-provider-supercargo//modules/gateway"
  project_id     = var.project_id
  region         = var.region
  product_id     = "customer-orders"
  image          = "europe-west1-docker.pkg.dev/supercargo-dev/images/gateway:latest"
  hub_address    = "https://hub-ezwd7z77ha-ew.a.run.app"
  kms_address    = "https://vault-ezwd7z77ha-ew.a.run.app"
  master_key_uri = "projects/supercargo-dev/locations/europe-west1/keyRings/supercargo-vault-keyring/cryptoKeys/supercargo-vault-master-key"

  # Declarative authorization of application service accounts
  authorized_invoker_service_accounts = [
    "order-service@supercargo-prod.iam.gserviceaccount.com",
    "fulfillment-worker@supercargo-prod.iam.gserviceaccount.com"
  ]

  # Additional IAM members authorized for Cloud Run invocation
  authorized_invokers = [
    "group:sre-team@example.com"
  ]
}
```

## Key Capabilities
- **High-Throughput Ingestion Enforcement**: Provisions Cloud Run v2 service running the Supercargo Gateway, connected to Pub/Sub push pipelines and BigQuery streaming sinks.
- **Declarative IAM Invocation Controls**: Exposes `authorized_invoker_service_accounts` (auto-formatted service account emails) and `authorized_invokers` (IAM member expressions) for granular `roles/run.invoker` authorization.
- **Dedicated Push Invoker Identity**: Provisions an isolated Pub/Sub push service account (`push_invoker_service_account_email`) with `roles/run.invoker` binding to securely forward raw Pub/Sub messages to the Gateway.
- **Resilient Pub/Sub Topologies**: Provisions Raw, Clean, and DLQ Pub/Sub topics with exponential backoff retry policies and Dead Letter Queue subscriptions (`raw_subscription_id`, `dlq_subscription_id`).
- **Dynamic BigQuery Storage**: Creates strongly typed BigQuery tables for registered contract schemas with deletion protection.

## Configuration Variables
| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `project_id` | `string` | *required* | The GCP project ID where resources are deployed. |
| `region` | `string` | *required* | The GCP region for Cloud Run and Pub/Sub resources. |
| `product_id` | `string` | *required* | The ID of the Data Product (used for naming resources). |
| `image` | `string` | *required* | The container image for the Gateway service. |
| `hub_address` | `string` | *required* | The address/URL of the Hub service. |
| `kms_address` | `string` | *required* | The address/URL of the KMS (Vault) service. |
| `master_key_uri` | `string` | *required* | The URI of the Master Key in GCP KMS for envelope encryption. |
| `authorized_invoker_service_accounts` | `list(string)` | `[]` | List of service account emails authorized to invoke the Gateway service (auto-prefixed with `serviceAccount:`). |
| `authorized_invokers` | `list(string)` | `[]` | List of raw IAM members (`user:...`, `group:...`, `serviceAccount:...`) authorized to invoke the Gateway. |
| `ingress_type` | `string` | `"INGRESS_TRAFFIC_ALL"` | Allowed ingress traffic type (`INGRESS_TRAFFIC_ALL`, `INGRESS_TRAFFIC_INTERNAL_ONLY`, etc.). |
| `auth_enforce` | `bool` | `true` | Whether to enforce OIDC token authentication at runtime (Shadow Mode if false). |
| `bigquery_dataset_id` | `string` | `""` | Optional BigQuery dataset ID for validated data sink tables. |
| `bigquery_deletion_protection` | `bool` | `true` | Whether to enable deletion protection on BigQuery tables. |
| `pubsub_max_delivery_attempts` | `number` | `5` | Maximum number of delivery attempts before pushing to DLQ. |
| `pubsub_minimum_backoff` | `string` | `"30s"` | Minimum backoff duration for Pub/Sub retries. |
| `pubsub_maximum_backoff` | `string` | `"600s"` | Maximum backoff duration for Pub/Sub retries. |

## Module Outputs
| Output | Type | Description |
| :--- | :--- | :--- |
| `service_name` | `string` | The name of the Gateway Cloud Run service. |
| `service_url` | `string` | The HTTPS URL of the Gateway service. |
| `raw_topic_id` | `string` | The ID of the Raw Pub/Sub topic. |
| `clean_topic_id` | `string` | The ID of the Clean Pub/Sub topic. |
| `dlq_topic_id` | `string` | The ID of the Dead Letter Queue Pub/Sub topic. |
| `push_invoker_service_account_email` | `string` | The email of the Pub/Sub Push Invoker Service Account. |
| `raw_subscription_id` | `string` | The ID of the Raw push subscription triggering Cloud Run. |
| `dlq_subscription_id` | `string` | The ID of the Dead Letter Queue subscription. |
| `service_account_email` | `string` | The email of the Gateway Service Account. |
| `bigquery_table_ids` | `map(string)` | Map of contract IDs to their destination BigQuery table IDs. |

## Implementation
- Source Code: [modules/gateway](../../modules/gateway)
