output "job_name" {
  description = "Name of the provisioned Cloud Run v2 Job."
  value       = google_cloud_run_v2_job.analytics_runner.name
}

output "job_id" {
  description = "Resource ID of the provisioned Cloud Run v2 Job."
  value       = google_cloud_run_v2_job.analytics_runner.id
}

output "runner_service_account_email" {
  description = "Service account email executing the analytics transformations."
  value       = local.runner_sa_email
}

output "scheduler_service_account_email" {
  description = "Service account email triggering the analytics job via Cloud Scheduler."
  value       = local.scheduler_sa_email
}
