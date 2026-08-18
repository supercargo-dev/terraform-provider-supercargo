---
type: Supercargo Terraform Module
title: vault Module
description: Terraform module for deploying the Supercargo Vault service and Cloud KMS configurations for PII encryption.
tags: [terraform, module, okf, kms, security]
---

# vault Module

Terraform module for deploying the Supercargo Vault service on Cloud Run and Google Cloud KMS configurations for PII encryption and DEK wrapping.

## Key Capabilities
- **Decoupled KMS Architecture**: Cloud KMS KeyRings and CryptoKeys use stable, deterministic naming decoupled from ephemeral random suffixes to prevent resource destruction attempts against immutable GCP KMS assets.
- **Native Key Rotation**: CryptoKey versions rotate in place according to `key_rotation_period` (default: 30 days) with `prevent_destroy = true` lifecycle safeguards.
- **Service Deployment**: Provisions Cloud Run v2 service for Vault with Secret Manager secrets, IAM role bindings, and Pub/Sub event topics.

## Configuration Variables
| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `keyring_name` | `string` | `"supercargo-vault-keyring"` | Name of the Cloud KMS KeyRing for the Vault. |
| `crypto_key_name` | `string` | `"supercargo-vault-master-key"` | Name of the Cloud KMS CryptoKey for the Vault master key. |
| `key_rotation_period` | `string` | `"2592000s"` | Rotation period for the Cloud KMS master key (30 days). |
| `project_id` | `string` | *required* | GCP project ID. |
| `region` | `string` | `"europe-west1"` | GCP region. |
| `service_name` | `string` | `"vault"` | Cloud Run service name. |
| `image` | `string` | *required* | Vault container image. |

## Implementation
- Source Code: [modules/vault](../../modules/vault)
