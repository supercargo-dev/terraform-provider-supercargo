# Analytics Runner Terraform Module (`analytics_runner`)

Reusable Terraform module provisioning an autonomous Google Cloud Run v2 Job and Google Cloud Scheduler trigger for data transformations (such as `dbt` or `sqlmesh`), with least-privilege BigQuery IAM bindings and deterministic service account email derivation.

## Architecture

This module provisions:
- **Cloud Run v2 Job (`google_cloud_run_v2_job.analytics_runner`)**: Executes the transformation runner container with command arguments `["analytics", "run", "--engine", var.engine, "--dialect", var.dialect]`, with configurable CPU, memory, timeout, and retries.
- **Deterministic Service Accounts**:
  - Runner service account: `${var.service_account_id != null && var.service_account_id != "" ? var.service_account_id : "${var.job_name}-sa"}@${var.project_id}.iam.gserviceaccount.com`
  - Scheduler service account: `${var.scheduler_service_account_id != null && var.scheduler_service_account_id != "" ? var.scheduler_service_account_id : "${var.job_name}-sched-sa"}@${var.project_id}.iam.gserviceaccount.com`
- **Cloud Scheduler Trigger (`google_cloud_scheduler_job.runner_schedule`)**: Periodically triggers the Cloud Run v2 Job via HTTP POST to the Cloud Run Admin API (`:run` endpoint) using OAuth 2.0 with the `https://www.googleapis.com/auth/cloud-platform` scope.
- **Least-Privilege IAM Bindings**:
  - `roles/run.invoker`: Granted to the Cloud Scheduler service account on the Cloud Run v2 Job.
  - `roles/bigquery.jobUser`: Granted at the project level to the runner service account to allow query execution.
  - `roles/bigquery.dataEditor`: Granted to the runner service account for each dataset in `var.target_datasets`.
  - `roles/bigquery.dataViewer`: Granted to the runner service account for each dataset in `var.input_datasets`.

## Usage

```hcl
module "analytics_runner" {
  source = "../../modules/analytics_runner"

  project_id      = "my-gcp-project"
  region          = "europe-west1"
  job_name        = "analytics-runner"
  container_image = "ghcr.io/supercargo-dev/supercargo-analytics-runner:latest"

  engine          = "dbt"
  dialect         = "bigquery"
  schedule        = "0 6 * * *"
  time_zone       = "Etc/UTC"

  input_datasets  = ["raw_events", "raw_crm"]
  target_datasets = ["analytics_mart", "analytics_staging"]

  cpu_limit       = "2"
  memory_limit    = "4Gi"
  timeout         = "1800s"
  max_retries     = 1
}
```

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| `project_id` | Google Cloud project ID | `string` | n/a | yes |
| `region` | Google Cloud region for the Cloud Run Job and Scheduler | `string` | n/a | yes |
| `job_name` | Name of the Cloud Run v2 Job | `string` | `"analytics-runner"` | no |
| `container_image` | Container image URI for the analytics runner job | `string` | n/a | yes |
| `service_account_id` | Custom account ID for the runner service account | `string` | `null` | no |
| `scheduler_service_account_id` | Custom account ID for the scheduler service account | `string` | `null` | no |
| `schedule` | Cron schedule expression for Cloud Scheduler | `string` | `"0 6 * * *"` | no |
| `time_zone` | Time zone for Cloud Scheduler | `string` | `"Etc/UTC"` | no |
| `engine` | Transformation engine to execute (`dbt`, `sqlmesh`) | `string` | `"dbt"` | no |
| `dialect` | SQL dialect / target warehouse (`bigquery`) | `string` | `"bigquery"` | no |
| `target_datasets` | BigQuery datasets where runner has `roles/bigquery.dataEditor` | `list(string)` | `[]` | no |
| `input_datasets` | BigQuery datasets where runner has `roles/bigquery.dataViewer` | `list(string)` | `[]` | no |
| `cpu_limit` | CPU limit for the container | `string` | `"2"` | no |
| `memory_limit` | Memory limit for the container | `string` | `"4Gi"` | no |
| `timeout` | Execution timeout for the Cloud Run job | `string` | `"1800s"` | no |
| `max_retries` | Number of retries for failed tasks | `number` | `1` | no |
| `paused` | Whether the Cloud Scheduler job is paused | `bool` | `false` | no |
| `deletion_protection` | Whether to protect Cloud Run Job from deletion | `bool` | `false` | no |

## Outputs

| Name | Description |
|------|-------------|
| `job_name` | Name of the provisioned Cloud Run v2 Job |
| `job_id` | Resource ID of the provisioned Cloud Run v2 Job |
| `runner_service_account_email` | Service account email executing analytics transformations |
| `scheduler_service_account_email` | Service account email triggering the job via Cloud Scheduler |
