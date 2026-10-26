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
| `contracts.*.schema` | `string` | no | no | yes | no | BigQuery JSON schema. |
| `contracts.*.version` | `string` | no | no | yes | no | Contract semantic version. |
| `id` | `string` | no | yes | yes | no | The URN of the Data Product. Optional if manifest_file is provided. |
| `location` | `string` | no | no | yes | no | BigQuery dataset location resolved from the Data Product output ports. |
| `manifest_file` | `string` | no | yes | no | no | Path to the local product.yaml manifest file. |
| `partition_expiration_ms` | `int64` | no | no | yes | no | BigQuery partition expiration in milliseconds. |
| `partitioning_field` | `string` | no | no | yes | no | BigQuery partitioning field resolved from the Data Product output ports. |
| `project` | `string` | no | yes | yes | no | GCP Project ID override. |
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
