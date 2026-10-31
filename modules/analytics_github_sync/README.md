# Analytics GitHub Sync Terraform Module (`analytics_github_sync`)

Turnkey Terraform module provisioning an autonomous Google Cloud Run v2 Job and Google Cloud Scheduler trigger for topic-based GitHub repository ingestion into BigQuery, fully compatible with declarative `dlt://` input ports and the unified runner image `ghcr.io/supercargo-dev/supercargo-dlt-runner`.

## Architecture

This module provisions:
- **Cloud Run v2 Job (`google_cloud_run_v2_job.sync_job`)**: Runs the packaged containerized `dlt` pipeline or the unified `supercargo-dlt-runner` (`ghcr.io/supercargo-dev/supercargo-dlt-runner`) in BigQuery mode with autonomous `--topic` repository discovery.
- **Deterministic Service Accounts**: Evaluates service account emails deterministically via `locals` (`${account_id}@${project_id}.iam.gserviceaccount.com`) to avoid Terraform plan-time cycle/dependency issues.
- **Cloud Scheduler (`google_cloud_scheduler_job.sync_schedule`)**: Triggers the Cloud Run Admin API (`:run` endpoint) via POST with an OAuth token possessing the `https://www.googleapis.com/auth/cloud-platform` scope.
- **Cloud Run Job Invoker IAM (`google_cloud_run_v2_job_iam_member.scheduler_invoker`)**: Grants `roles/run.invoker` strictly to the Cloud Scheduler service account.
- **BigQuery IAM (`google_project_iam_member`)**: Grants `roles/bigquery.dataEditor` and `roles/bigquery.jobUser` to the sync service account.
- **Secret Manager IAM (`google_secret_manager_secret_iam_member`)**: Grants `roles/secretmanager.secretAccessor` for the GitHub token secret to the sync service account.
- **Environment & Secrets Injection**: Injects `SUPERCARGO_VAULT_URL`, `SUPERCARGO_VAULT_AUDIENCE`, `SUPERCARGO_HUB_URL`, `SUPERCARGO_HUB_AUDIENCE`, `GOOGLE_CLOUD_PROJECT`, `DESTINATION__BIGQUERY__LOCATION`, `GITHUB_TOKEN`, and `SUPERCARGO_CREDENTIALS_SECRET` (via `secret_key_ref`).
- **Lifecycle Decoupling**: Configures `lifecycle { ignore_changes = [template[0].template[0].containers[0].image, client, client_version] }` so CI/CD image promotions do not trigger drift or rollbacks during Terraform applies.

## Usage

```hcl
module "analytics_github_sync" {
  source = "../../modules/analytics_github_sync"

  project_id              = "my-gcp-project"
  region                  = "europe-west1"
  job_name                = "analytics-github-sync"
  container_image         = "ghcr.io/supercargo-dev/supercargo-dlt-runner:latest"
  org                     = "supercargo-dev"
  topic                   = "supercargo-analytics"
  dataset                 = "supercargo_github"
  github_token_secret_id  = "github-token"
  vault_url               = "https://vault.internal.supercargo.net"
  vault_audience          = "https://vault.internal.supercargo.net"
  hub_url                 = "https://hub.internal.supercargo.net"
  hub_audience            = "https://hub.internal.supercargo.net"
  schedule                = "0 * * * *"
  deletion_protection     = false
}
```

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| `project_id` | Google Cloud project ID | `string` | n/a | yes |
| `region` | Google Cloud region | `string` | `"europe-west1"` | no |
| `job_name` | Name of the Cloud Run v2 Job | `string` | `"analytics-github-sync"` | no |
| `container_image` | Container image URI for GitHub sync | `string` | n/a | yes |
| `org` | GitHub organization name | `string` | n/a | yes |
| `topic` | GitHub topic for autonomous discovery | `string` | `"supercargo-analytics"` | no |
| `dataset` | Target BigQuery dataset name | `string` | `"supercargo_github"` | no |
| `vault_url` | Supercargo Vault service URL | `string` | `""` | no |
| `vault_audience` | Supercargo Vault OIDC audience | `string` | `""` | no |
| `hub_url` | Supercargo Hub service URL for runner asset health reporting | `string` | `""` | no |
| `hub_audience` | Supercargo Hub OIDC audience for authentication | `string` | `""` | no |
| `github_token_secret_id` | Secret Manager secret ID for GitHub token | `string` | n/a | yes |
| `service_account_id` | Account ID for sync service account | `string` | `"analytics-github-sync-sa"` | no |
| `scheduler_service_account_id` | Account ID for scheduler service account | `string` | `"analytics-github-sync-sched-sa"` | no |
| `schedule` | Cron schedule for Cloud Scheduler | `string` | `"0 * * * *"` | no |
| `time_zone` | Timezone for Cloud Scheduler | `string` | `"Etc/UTC"` | no |
| `scheduler_job_name` | Name of the Cloud Scheduler job | `string` | `null` | no |
| `scheduler_attempt_deadline` | Attempt deadline for Cloud Scheduler | `string` | `"320s"` | no |
| `paused` | Whether the Cloud Scheduler job is paused | `bool` | `false` | no |
| `deletion_protection` | Protect Cloud Run Job from deletion | `bool` | `false` | no |
| `cpu_limit` | CPU limit for the job container | `string` | `"1"` | no |
| `memory_limit` | Memory limit for the job container | `string` | `"1Gi"` | no |
| `timeout` | Execution timeout for the job | `string` | `"600s"` | no |
| `max_retries` | Number of retries for failed tasks | `number` | `1` | no |

## Outputs

| Name | Description |
|------|-------------|
| `job_name` | The name of the provisioned Cloud Run v2 Job |
| `job_id` | The full resource ID of the provisioned Cloud Run v2 Job |
| `service_account_email` | Deterministic service account email running the ingestion Cloud Run Job |
| `scheduler_job_name` | The name of the Cloud Scheduler job triggering ingestion |
