# Supercargo Terraform Provider

**Intent:** Custom Terraform provider code for managing Supercargo platform resources via Infrastructure as Code.

## Key Capabilities
- **Resource Management:** Terraform definitions for managing `DataProduct`, `ContractVersion`, `Team`, and `GatewayConfig` resources in the Supercargo Hub.
- **Data Sources:** Ability to query existing contracts and products from Terraform using data sources.
- **Provider Framework:** Built using the standard Hashicorp Terraform Plugin Framework.

## Knowledge Catalog
To learn more about the specific Terraform resources and modules provided by this package, consult the [Open Knowledge Format (OKF) Catalog](docs/catalog/index.md).

## Agent Constraints
- **Idempotency:** All resources defined here must be strictly idempotent when interacting with the Hub API.
