---
type: "Supercargo Terraform Provider"
title: "supercargo"
description: "Terraform provider schema for supercargo."
visibility: "customer"
tags: [terraform, provider, okf]
---

# supercargo

## Schema Attributes

| Attribute | Type | Required | Optional | Computed | Sensitive | Description |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `audience` | `string` | no | yes | no | no | Target OIDC audience for Hub authentication (defaults to https://<hub-host>). |
| `hub_address` | `string` | no | yes | no | no | Address of the Supercargo Hub (HubService). |
| `token` | `string` | no | yes | no | yes | Explicit OIDC ID token for authenticating with the Hub. If omitted, the provider will attempt to generate one automatically using Application Default Credentials. |
| `use_tls` | `bool` | no | yes | no | no | Whether to connect to the Hub using TLS. If omitted, defaults to false for localhost/loopback addresses and true for all other hosts. |

## Implementation
- Source Code: `internal/provider`
