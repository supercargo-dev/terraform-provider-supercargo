---
type: Supercargo Terraform Module
title: data_product Module
description: Terraform module for deploying Data Products and their Data Contracts into the Supercargo Hub.
tags: [terraform, module, okf]
---

# data_product Module

Terraform module for deploying Data Products and their Data Contracts into the Supercargo Hub.

## Golden Path Usage
```terraform
module "customer_telemetry" {
  source        = "github.com/supercargo-dev/terraform-provider-supercargo//modules/data_product"
  project_id    = var.project_id
  region        = var.region
  manifest_file = "${path.module}/product.yaml"
}
```

## Key Capabilities
- **Native Manifest Contract Resolution**: Auto-resolves contract files, schema JSONs, and content hashes from `product.yaml`.
- **Dynamic Table Wiring**: Dynamically supplies BigQuery table configurations to the underlying `gateway` module with normalized table identifiers.
- **Backwards Compatibility**: Supports explicit contract maps via `var.contracts` for CI/CD artifact-passing pipelines.

## Implementation
- Source Code: [modules/data_product](../../modules/data_product)
