---
type: "Supercargo Concept"
title: "Terraform Provider for Supercargo"
description: "Official Terraform provider for managing Supercargo Data Products, Contracts, and Core infrastructure."
visibility: "customer"
tags: [terraform, provider, okf]
---

# Terraform Provider for Supercargo

Welcome to the Knowledge Catalog for the Supercargo Terraform Provider. This provider allows data engineers and platform teams to manage Supercargo entities as Code using HashiCorp Terraform.

## Overview

The `terraform-provider-supercargo` bridges the declarative world of Terraform with the Supercargo Control Plane (Hub). It enables users to register data products, manage data contracts, configure gateways, and establish teams using standard HCL.

## Migration & Architecture Notes

Data Product and Gateway infrastructure are declaratively compiled via `modules/data_product` and `modules/gateway`. Mutable pseudo-resources (`supercargo_data_product`, `supercargo_contract_version`, `supercargo_gateway_config`, `supercargo_ingestion_gateway`) are retired in favor of declarative infrastructure compilation directly from `product.yaml`.

## Resources
* [supercargo_team](resource-team.md)

## Data Sources
* [supercargo_contract](data-source-contract.md)
* [supercargo_data_product](data-source-data-product.md)

## Provider
* [supercargo](provider.md)

## Modules
* [analytics_github_sync Module](module-analytics_github_sync.md)
* [data_product Module](module-data_product.md)
* [dataplex_taxonomy Module](module-dataplex_taxonomy.md)
* [docs_portal Module](module-docs_portal.md)
* [gateway Module](module-gateway.md)
* [gh-wif Module](module-gh-wif.md)
* [global_telemetry_share Module](module-global_telemetry_share.md)
* [hub Module](module-hub.md)
* [loader Module](module-loader.md)
* [metadata_shovel Module](module-metadata_shovel.md)
* [security_vault Module](module-security_vault.md)
* [vault Module](module-vault.md)

