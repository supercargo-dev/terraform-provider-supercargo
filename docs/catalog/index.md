---
type: Supercargo Concept
title: Terraform Provider for Supercargo
description: Official Terraform provider for managing Supercargo Data Products, Contracts, and Core infrastructure.
tags: [terraform, provider, okf]
---

# Terraform Provider for Supercargo

Welcome to the Knowledge Catalog for the Supercargo Terraform Provider. This provider allows data engineers and platform teams to manage Supercargo entities as Code using HashiCorp Terraform.

## Overview

The `terraform-provider-supercargo` bridges the declarative world of Terraform with the Supercargo Control Plane (Hub). It enables users to register data products, manage data contracts, configure gateways, and establish teams using standard HCL.

## Key Resources

* **`supercargo_data_product`**: Registers a new Data Product within the Supercargo Hub.
* **`supercargo_contract_version`**: Publishes a new version of a Data Contract, attaching SLA and Health metadata.
* **`supercargo_team`**: Defines a Team within the Supercargo platform for ownership and RBAC.
* **`supercargo_gateway_config`**: Configures data routing rules and policy enforcement for a specific Gateway instance.
* **`supercargo_ingestion_gateway`**: Deploys and configures an Ingestion Gateway for a Data Product.

## Usage

This provider is intended to be used within the deployment pipelines of individual Data Products (Spokes) as well as by the core Platform team managing the Hub.

When you run `sc apply`, the CLI under the hood orchestrates Terraform execution using this provider to push the declarative state defined in your `product.yaml` to the remote Supercargo Hub.

## Resources
* [supercargo_data_product](resource-data-product.md)
* [supercargo_contract_version](resource-contract-version.md)
* [supercargo_team](resource-team.md)
* [supercargo_gateway_config](resource-gateway-config.md)
* [supercargo_ingestion_gateway](resource-ingestion-gateway.md)

## Modules
* [data_product Module](module-data_product.md)
* [gateway Module](module-gateway.md)
* [gh-wif Module](module-gh-wif.md)
* [hub Module](module-hub.md)
* [loader Module](module-loader.md)
* [metadata_shovel Module](module-metadata_shovel.md)
* [security_vault Module](module-security_vault.md)
* [vault Module](module-vault.md)
