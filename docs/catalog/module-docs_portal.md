---
type: "Supercargo Terraform Module"
title: "docs_portal Module"
description: "Terraform module for deploying and configuring docs_portal."
visibility: "customer"
tags: [terraform, module, okf]
---

# docs_portal Module

Terraform module for deploying and configuring `docs_portal`.

## Architecture & Data Flow

```mermaid
flowchart LR
    Inputs["Module Inputs"] --> Module["docs_portal Module"]
    Module --> Resources["Managed Resources"]
    Module --> Outputs["Module Outputs"]
```

## Golden Path Usage

```terraform
module "docs_portal" {
  source = "github.com/supercargo-dev/terraform-provider-supercargo//modules/docs_portal"
  image                    = var.image
  project_id               = var.project_id
}
```

## Configuration Variables

| Variable | Type | Default | Sensitive | Description |
| :--- | :--- | :--- | :--- | :--- |
| `domain_name` | `string` | `""` | no | The fully qualified custom domain name for the docs portal (e.g., docs.example.com). Leave empty to use Cloud Run default URI. |
| `iap_client_id` | `string` | `""` | no | Optional OAuth 2.0 Client ID for Identity-Aware Proxy (IAP) |
| `iap_client_secret` | `string` | (sensitive) | yes | Optional OAuth 2.0 Client Secret for Identity-Aware Proxy (IAP) |
| `iap_enabled` | `bool` | `true` | no | Whether to enable Identity-Aware Proxy (IAP) directly on the Cloud Run service |
| `image` | `string` | *required* | no | The container image to deploy to Cloud Run |
| `licensed_domains` | `list(string)` | `[]` | no | List of corporate domains authorized to access the docs portal via IAP (e.g., ['example.com'] or ['domain:example.com']) |
| `project_id` | `string` | *required* | no | The GCP project ID where resources will be provisioned |
| `region` | `string` | `"europe-west1"` | no | The GCP region for regional resources (Cloud Run, Domain Mapping) |
| `service_name` | `string` | `"docs-portal"` | no | The name of the Cloud Run service |

## Module Outputs

| Output | Sensitive | Description |
| :--- | :--- | :--- |
| `cloud_run_uri` | no | The default *.run.app URI generated for the Cloud Run service |
| `domain_mapping_dns_records` | no | DNS resource records (A, AAAA, CNAME) required by DNS registrar for custom domain mapping |
| `service_url` | no | The authoritative HTTPS URL for the docs portal |

## Implementation
- Source Code: `modules/docs_portal`
