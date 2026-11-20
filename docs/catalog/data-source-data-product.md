---
type: "Supercargo Terraform Data Source"
title: "supercargo_data_product"
description: "Retrieves metadata, BigQuery physical configuration, service identities, SLA, and data contracts for a Supercargo Data Product."
visibility: "customer"
tags: [terraform, data-source, okf]
---

# supercargo_data_product

Retrieves metadata, BigQuery physical configuration, service identities, SLA, and data contracts for a Supercargo Data Product.

## Schema Attributes

| Attribute | Type | Required | Optional | Computed | Sensitive | Description |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `contracts` | `map(object)` | no | no | yes | no | Map of resolved data contracts keyed by contract URN or port name. |
| `contracts.*.commit_sha` | `string` | no | no | yes | no | Git commit SHA of the contract. |
| `contracts.*.content_hash` | `string` | no | no | yes | no | SHA256 content hash of the contract. |
| `contracts.*.id` | `string` | no | no | yes | no | Contract URN. |
| `contracts.*.schema_json` | `string` | no | no | yes | no | BigQuery JSON schema. |
| `contracts.*.version` | `string` | no | no | yes | no | Contract semantic version. |
| `id` | `string` | no | yes | yes | no | The URN of the Data Product. Optional if manifest_file is provided. |
| `include_dlt_metadata` | `bool` | no | yes | no | no | Whether to append _dlt_load_id and _dlt_id STRING REQUIRED columns to generated contract schemas. Defaults to false. |
| `ingestion_engine` | `string` | no | yes | no | no | BigQuery ingestion engine override: 'streaming' (default) or 'dlt' (maps repeated fields to JSON NULLABLE). If omitted, inferred per port from manifest and contract metadata. |
| `inputs` | `map(object)` | no | no | yes | no | Map of compiled boundary input landing tables keyed by contract URN or port name. |
| `inputs.*.name` | `string` | no | no | yes | no | Input port name. |
| `inputs.*.record_name` | `string` | no | no | yes | no | Input record name. |
| `inputs.*.schema_json` | `string` | no | no | yes | no | BigQuery physical schema JSON for base landing table with policy tags. |
| `inputs.*.source` | `string` | no | no | yes | no | Ingress data source URI (e.g. pubsub://managed, dlt://...). |
| `inputs.*.table_name` | `string` | no | no | yes | no | BigQuery landing table name in the internal dataset. |
| `inputs.*.urn` | `string` | no | no | yes | no | Internal ingress contract URN. |
| `internal_dataset_id` | `string` | no | no | yes | no | BigQuery dataset ID for private internal landing tables. |
| `location` | `string` | no | no | yes | no | BigQuery dataset location resolved from the Data Product output ports. |
| `lock_dir` | `string` | no | yes | yes | no | Optional explicit path to the compiled .supercargo/lock directory. If omitted, auto-discovered. |
| `manifest_file` | `string` | no | yes | no | no | Path to the local product.yaml manifest file. |
| `outputs` | `map(object)` | no | no | yes | no | Map of compiled output views keyed by contract URN or port name. |
| `outputs.*.lens` | `string` | no | no | yes | no | Relational lens applied (e.g. replica, event_log, scd2, cdc, reidentified). |
| `outputs.*.materialization` | `string` | no | no | yes | no | Materialization type (view or table). |
| `outputs.*.name` | `string` | no | no | yes | no | Output port name. |
| `outputs.*.urn` | `string` | no | no | yes | no | Output contract URN. |
| `outputs.*.view_query` | `string` | no | no | yes | no | Unwrapped GoogleSQL query body for authorized view definition. |
| `outputs.*.view_sql` | `string` | no | no | yes | no | Full GoogleSQL CREATE VIEW DDL statement. |
| `partition_expiration_ms` | `int64` | no | no | yes | no | BigQuery partition expiration in milliseconds. |
| `partitioning_field` | `string` | no | no | yes | no | BigQuery partitioning field resolved from the Data Product output ports. |
| `project` | `string` | no | yes | yes | no | GCP Project ID override. |
| `public_dataset_id` | `string` | no | no | yes | no | BigQuery dataset ID for public authorized views. |
| `service_identities` | `map(string)` | no | no | yes | no | Map of component identities (e.g. gateway, shovel) as IAM member strings. |
| `sla` | `object` | no | no | yes | no | Service Level Agreement (SLA) for the data product. |
| `sla.freshness` | `string` | no | no | yes | no | Target data freshness window (e.g., '15m', '1h'). |
| `sla.latency` | `string` | no | no | yes | no | Maximum ingestion latency. |
| `sla.rto` | `string` | no | no | yes | no | Recovery Time Objective (e.g., '4h', '24h'). |
| `sla.tier` | `string` | no | no | yes | no | Criticality tier (e.g., SLA_TIER_1_MISSION_CRITICAL). |
| `urn` | `string` | no | no | yes | no | Globally unique identifier (URN) of the Data Product. |
| `version` | `string` | no | yes | yes | no | The version of the Data Product. If omitted, latest active version will be retrieved. |

## Implementation
- Source Code: `internal/provider`
