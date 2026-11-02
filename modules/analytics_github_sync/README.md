# Analytics GitHub Sync Terraform Module (`analytics_github_sync`)

Turnkey Terraform module provisioning autonomous Google Cloud Run v2 Jobs and Google Cloud Scheduler triggers for partitioned GitHub repository ingestion into BigQuery, fully compatible with declarative `dlt://` input ports and the unified runner image `ghcr.io/supercargo-dev/supercargo-dlt-runner`.

## Architecture

This module provisions:
- **Per-Contract Cloud Run v2 Jobs (`google_cloud_run_v2_job.sync_jobs`)**: Runs containerized `dlt` pipeline jobs partitioned across 5 distinct data contracts:
  - `repositories`: Ingests `labeled_repositories`
  - `issues`: Ingests `issues`
  - `pulls`: Ingests `pulls`
  - `workflow_runs`: Ingests `workflow_runs`
  - `billing`: Ingests `org_metered_usage`
- **Deterministic Service Accounts**: Evaluates service account emails deterministically via `locals` (`${account_id}@${project_id}.iam.gserviceaccount.com`) to avoid Terraform plan-time cycle/dependency issues.
- **Per-Contract Cloud Schedulers (`google_cloud_scheduler_job.sync_schedules`)**: Triggers each Cloud Run Admin API (`:run` endpoint) via POST with an OAuth token possessing the `https://www.googleapis.com/auth/cloud-platform` scope on staggered default schedules (`0 1 * * *` through `0 5 * * *`), overridable via `schedules`.
- **Cloud Run Job Invoker IAM (`google_cloud_run_v2_job_iam_member.scheduler_invoker`)**: Grants `roles/run.invoker` strictly to the Cloud Scheduler service account per job.
- **BigQuery IAM**: Grants `roles/bigquery.dataEditor` scoped strictly to the target dataset via `google_bigquery_dataset_iam_member.sync_job_bq_editor`, while `roles/bigquery.jobUser` is granted at the project level via `google_project_iam_member.sync_job_bq_user` for query job execution.
- **Secret Manager IAM (`google_secret_manager_secret_iam_member`)**: Grants `roles/secretmanager.secretAccessor` for the GitHub token secret to the sync service account.
- **Environment & Secrets Injection**: Injects `SUPERCARGO_VAULT_URL`, `SUPERCARGO_VAULT_AUDIENCE`, `SUPERCARGO_HUB_URL`, `SUPERCARGO_HUB_AUDIENCE`, `GOOGLE_CLOUD_PROJECT`, `DESTINATION__BIGQUERY__LOCATION`, `GITHUB_TOKEN`, and `SUPERCARGO_CREDENTIALS_SECRET` (via `secret_key_ref`).
- **Lifecycle Decoupling**: Configures `lifecycle { ignore_changes = [template[0].template[0].containers[0].image, client, client_version] }` so CI/CD image promotions do not trigger drift or rollbacks during Terraform applies.
- **Seamless State Migration**: Includes declarative `moved` blocks migrating legacy `google_cloud_run_v2_job.sync_job` and `google_cloud_scheduler_job.sync_schedule` into `sync_jobs["repositories"]` and `sync_schedules["repositories"]` without resource recreation.

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
  deletion_protection     = false
}
```

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| `project_id` | Google Cloud project ID | `string` | n/a | yes |
| `region` | Google Cloud region | `string` | `"europe-west1"` | no |
| `job_name` | Base name of the Cloud Run v2 Jobs | `string` | `"analytics-github-sync"` | no |
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
| `schedule` | Default cron schedule for Cloud Scheduler (legacy fallback) | `string` | `"0 * * * *"` | no |
| `schedules` | Optional map of cron schedules overriding defaults per contract (`repositories`, `issues`, `pulls`, `workflow_runs`, `billing`) | `map(string)` | `{}` | no |
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
| `job_name` | Primary Cloud Run v2 Job name (deprecated: use `job_names`) |
| `job_id` | Primary Cloud Run v2 Job resource ID (deprecated: use `job_ids`) |
| `job_names` | Map of contract names to provisioned Cloud Run v2 Job names (`job_names`) |
| `job_ids` | Map of contract names to provisioned Cloud Run v2 Job resource IDs |
| `service_account_email` | Deterministic service account email running the ingestion Cloud Run Jobs |
| `scheduler_job_name` | Primary Cloud Scheduler job name (deprecated: use `scheduler_job_names`) |
| `scheduler_job_names` | Map of contract names to provisioned Cloud Scheduler job names (`scheduler_job_names`) |

