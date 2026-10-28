# Docs Portal Terraform Module (`docs_portal`)

Turnkey, production-hardened Terraform module provisioning an autonomous Google Cloud Run v2 documentation portal fronted by Google Cloud Load Balancing (GCLB) with Google Identity-Aware Proxy (IAP) and automated HTTP-to-HTTPS redirection.

## Architecture

This module provisions:
- **Cloud Run v2 Service (`google_cloud_run_v2_service.docs_portal`)**: Runs the containerized static documentation server.
  - `ingress = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"`: Hard-blocks direct public internet traffic at the Google edge; only traffic routed through the authorized internal/external load balancer is accepted.
  - Resource allocation: 256Mi memory, 1 CPU by default.
  - Health checking: Dedicated `startup_probe` and `liveness_probe` querying `GET /healthz` on port 8080.
- **Cloud Run Invoker IAM (`google_cloud_run_v2_service_iam_member.all_users_invoker`)**: Grants `roles/run.invoker` to `allUsers`.
  - *Cloud Physics Rationale*: Under Google Cloud's serverless architecture, the GCLB Serverless Network Endpoint Group (NEG) does not sign requests with OIDC identity tokens or assume the IAP service agent identity. Identity verification and access policy enforcement happen exclusively at the GCLB Backend Service via IAP before requests reach Cloud Run. Direct public invocation of the Cloud Run URL (`*.run.app`) is forbidden by `INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER`.
- **Serverless NEG (`google_compute_region_network_endpoint_group.serverless_neg`)**: Bridges the global load balancing fabric to the regional Cloud Run service.
- **Backend Service with IAP (`google_compute_backend_service.backend`)**: External HTTP(S) backend service wiring the serverless NEG and configuring Google Identity-Aware Proxy (IAP) with OAuth 2.0 credentials.
- **Plan-Time Deterministic IAP IAM (`google_iap_web_backend_service_iam_member.licensed_domains`)**: Normalizes licensed corporate domains (ensuring `domain:` prefix) and binds `roles/iap.httpsResourceAccessor` via plan-time deterministic `for_each = local.normalized_licensed_domains`.
- **Global HTTPS Load Balancing Fabric**:
  - Static Global IP (`google_compute_global_address.docs_portal`)
  - Google-Managed SSL Certificate (`google_compute_managed_ssl_certificate.docs_portal`)
  - Target HTTPS Proxy (`google_compute_target_https_proxy.docs_portal`)
  - HTTPS Forwarding Rule on port 443 (`google_compute_global_forwarding_rule.docs_portal_https`)
- **HTTP-to-HTTPS Redirection Fabric**:
  - Redirect URL Map (`google_compute_url_map.https_redirect`) with `https_redirect = true`
  - Target HTTP Proxy (`google_compute_target_http_proxy.https_redirect`)
  - HTTP Forwarding Rule on port 80 (`google_compute_global_forwarding_rule.docs_portal_http`)

## Usage

```hcl
module "docs_portal" {
  source = "../../modules/docs_portal"

  project_id        = "my-gcp-project"
  region            = "europe-west1"
  service_name      = "docs-portal"
  image             = "europe-west1-docker.pkg.dev/my-gcp-project/supercargo/docs-portal:latest"
  domain_name       = "docs.example.com"
  licensed_domains  = ["example.com"]
  iap_enabled       = true
  iap_client_id     = "123456789-abcdef.apps.googleusercontent.com"
  iap_client_secret = "GOCSPX-secretkey"
}
```

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| `project_id` | The GCP project ID where resources will be provisioned | `string` | n/a | yes |
| `region` | The GCP region for regional resources | `string` | `"europe-west1"` | no |
| `service_name` | The name of the Cloud Run service and base naming prefix | `string` | `"docs-portal"` | no |
| `image` | The container image to deploy to Cloud Run | `string` | n/a | yes |
| `domain_name` | The fully qualified custom domain name for the docs portal | `string` | n/a | yes |
| `licensed_domains` | List of corporate domains authorized to access via IAP | `list(string)` | `[]` | no |
| `iap_enabled` | Whether to enable Identity-Aware Proxy (IAP) on backend service | `bool` | `true` | no |
| `iap_client_id` | OAuth 2.0 Client ID for Identity-Aware Proxy | `string` | `""` | no |
| `iap_client_secret` | OAuth 2.0 Client Secret for Identity-Aware Proxy | `string` | `""` | no |

## Outputs

| Name | Description |
|------|-------------|
| `load_balancer_ip` | The static global external IP address assigned to the Cloud Load Balancer |
| `service_url` | The authoritative HTTPS URL for the docs portal |
| `backend_service_id` | The ID of the GCLB backend service |
