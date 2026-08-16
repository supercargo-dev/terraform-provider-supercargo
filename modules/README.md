# Terraform Modules (`modules/`)

**Intent:** Reusable, parameterized Terraform modules for provisioning Supercargo services and infrastructure components.

## Key Capabilities
- **Standardization:** Encapsulates best practices (e.g., IAM, Cloud Run configs, h2c enablement) into reusable blocks.
- **Service Provisioning:** Dedicated modules for each core microservice, ensuring consistent deployments across environments.

## Directory Map
- `gateway/`: Provisions the Gateway service (Enforcement Plane).
- `hub/`: Provisions the Hub service (Control Plane).
- `loader/`: Provisions the Loader service (Batch Ingress).
- `metadata_shovel/`: Provisions the Event-driven metadata relay.
- `security_vault/`: Base security infrastructure for keys and secrets.
- `vault/`: Provisions the Vault service (Security Plane).
- `gh-wif/`: Provisions Workload Identity Federation for GitHub Actions.
