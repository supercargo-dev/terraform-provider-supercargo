---
type: "Supercargo Terraform Data Source"
title: "supercargo_contract"
description: "Terraform data_source schema for supercargo_contract."
visibility: "customer"
tags: [terraform, data-source, okf]
---

# supercargo_contract

## Schema Attributes

| Attribute | Type | Required | Optional | Computed | Sensitive | Description |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `fields` | `list(object)` | no | no | yes | no | The flattened schema fields of the contract. |
| `fields.*.description` | `string` | no | no | yes | no | - |
| `fields.*.mode` | `string` | no | no | yes | no | - |
| `fields.*.name` | `string` | no | no | yes | no | - |
| `fields.*.type` | `string` | no | no | yes | no | - |
| `id` | `string` | yes | no | no | no | The URN of the Data Contract. |
| `owner_team` | `string` | no | no | yes | no | The team that owns this contract. |
| `schema_json` | `string` | no | no | yes | no | The generated BigQuery JSON schema definition. |
| `urn` | `string` | no | no | yes | no | The exact URN of the contract. |
| `version` | `string` | no | yes | yes | no | The version of the Data Contract. If omitted, the latest active version will be retrieved. |

## Implementation
- Source Code: `internal/provider`
