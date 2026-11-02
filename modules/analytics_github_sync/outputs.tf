output "job_name" {
  description = "Primary Cloud Run v2 Job name (deprecated: use job_names)."
  value       = google_cloud_run_v2_job.sync_jobs["repositories"].name
}

output "job_id" {
  description = "Primary Cloud Run v2 Job resource ID (deprecated: use job_ids)."
  value       = google_cloud_run_v2_job.sync_jobs["repositories"].id
}

output "job_names" {
  description = "Map of contract names to provisioned Cloud Run v2 Job names."
  value       = { for k, v in google_cloud_run_v2_job.sync_jobs : k => v.name }
}

output "job_ids" {
  description = "Map of contract names to provisioned Cloud Run v2 Job resource IDs."
  value       = { for k, v in google_cloud_run_v2_job.sync_jobs : k => v.id }
}

output "service_account_email" {
  description = "Deterministic service account email running the ingestion Cloud Run Jobs."
  value       = local.sa_email
}

output "scheduler_job_name" {
  description = "Primary Cloud Scheduler job name (deprecated: use scheduler_job_names)."
  value       = google_cloud_scheduler_job.sync_schedules["repositories"].name
}

output "scheduler_job_names" {
  description = "Map of contract names to provisioned Cloud Scheduler job names."
  value       = { for k, v in google_cloud_scheduler_job.sync_schedules : k => v.name }
}

