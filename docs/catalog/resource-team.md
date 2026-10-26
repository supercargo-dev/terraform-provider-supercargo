---
type: "Supercargo Terraform Resource"
title: "supercargo_team"
description: "Terraform resource schema for supercargo_team."
visibility: "customer"
tags: [terraform, resource, okf]
---

# supercargo_team

## Schema Attributes

| Attribute | Type | Required | Optional | Computed | Sensitive | Description |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `data_asset` | `string` | yes | no | no | no | Reference to the authoritative source (e.g., 'go://...'). |
| `members` | `list(string)` | no | yes | no | no | List of User Principals (emails) or Group Principals belonging to this team. |
| `metadata` | `map(string)` | no | yes | no | no | Arbitrary metadata for the team. |
| `name` | `string` | yes | no | no | no | Unique slug for the team (e.g., 'mobile-analytics'). |
| `urn` | `string` | no | no | yes | no | Globally unique identifier (URN). |

## Implementation
- Source Code: `internal/provider`
