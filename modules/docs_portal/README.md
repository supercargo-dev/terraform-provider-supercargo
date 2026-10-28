# Docs Portal Terraform Module (`docs_portal`)

Turnkey, zero-idle-cost Terraform module provisioning an autonomous Google Cloud Run v2 documentation portal with **Direct Identity-Aware Proxy (IAP)** integration and direct Custom Domain Mapping.

## Architecture

This module provisions:
- **Cloud Run v2 Service (`google_cloud_run_v2_service.docs_portal`)**: Runs the containerized documentation server with true scale-to-zero.
  - `ingress = "INGRESS_TRAFFIC_ALL"`: Direct Google Edge ingress.
  - `launch_stage = "BETA"`: Direct IAP support on Cloud Run.
  - `iap_enabled = var.iap_enabled`: Direct Google IAP authentication enforced at Google Edge frontends before requests invoke the container.
  - Resource allocation: 256Mi memory, 1 CPU by default.
  - Health checking: Dedicated `startup_probe` and `liveness_probe` querying `GET /healthz` on port 8080.
  - Lifecycle: Ignores changes to container image and client version to decouple CI/CD container delivery from infrastructure management.
- **IAP Service Agent Invoker IAM (`google_cloud_run_v2_service_iam_member.iap_service_agent_invoker`)**: 
  - When `iap_enabled = true`, binds `roles/run.invoker` strictly to the Google Cloud IAP service agent (`service-[PROJECT_NUMBER]@gcp-sa-iap.iam.gserviceaccount.com`).
  - *Zero-Bypass Security Invariant*: Unauthenticated callers cannot bypass IAP to invoke Cloud Run directly. Only traffic authenticated and forwarded by the IAP service agent is authorized to invoke the container.
  - When `iap_enabled = false`, falls back to public invocation via `allUsers`.
- **Domain IAM Access (`google_iap_web_cloud_run_service_iam_member.licensed_domains`)**:
  - Normalizes licensed corporate domains (`domain:<domain>`) and grants `roles/iap.httpsResourceAccessor` directly on the Cloud Run service.
- **Custom Domain Mapping (`google_cloud_run_domain_mapping.docs_portal`)**:
  - Maps `var.domain_name` (e.g., `docs.supercargo.dev`) directly to the Cloud Run service.
  - Automated Google-managed TLS certificates with zero load balancer costs ($0/mo base cost).

## Cost Comparison

| Feature | Legacy GCLB + IAP | Direct IAP on Cloud Run |
| :--- | :--- | :--- |
| **Idle Base Cost** | ~$18 - $20 / month (Forwarding rules) | **$0.00 / month** (Scales to zero) |
| **Load Balancer** | Global External HTTPS LB required | None (Google Edge native) |
| **Authentication** | IAP on Backend Service | Direct IAP on Cloud Run |
| **Domain IAM** | `roles/iap.httpsResourceAccessor` | `roles/iap.httpsResourceAccessor` |

## Usage

```hcl
module "docs_portal" {
  source = "../../modules/docs_portal"

  project_id       = "my-gcp-project"
  region           = "europe-west1"
  service_name     = "docs-portal"
  image            = "europe-west1-docker.pkg.dev/my-gcp-project/supercargo/docs-portal:latest"
  domain_name      = "docs.example.com"
  licensed_domains = ["example.com"]
  iap_enabled      = true
}
```

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| `project_id` | The GCP project ID where resources will be provisioned | `string` | n/a | yes |
| `region` | The GCP region for regional resources | `string` | `"europe-west1"` | no |
| `service_name` | The name of the Cloud Run service | `string` | `"docs-portal"` | no |
| `image` | The container image to deploy to Cloud Run | `string` | n/a | yes |
| `domain_name` | The fully qualified custom domain name (leave empty to use Cloud Run default URI) | `string` | `""` | no |
| `licensed_domains` | List of corporate domains authorized to access via IAP | `list(string)` | `[]` | no |
| `iap_enabled` | Whether to enable Identity-Aware Proxy (IAP) directly on Cloud Run | `bool` | `true` | no |

## Outputs

| Name | Description |
|------|-------------|
| `service_url` | The authoritative HTTPS URL for the docs portal |
| `cloud_run_uri` | The default `*.run.app` URI generated for the Cloud Run service |
| `domain_mapping_dns_records` | DNS resource records required by DNS registrar for custom domain |
