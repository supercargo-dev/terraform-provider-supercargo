output "job_name" {
  description = "The name of the provisioned Cloud Run v2 Job."
  value       = google_cloud_run_v2_job.sync_job.name
}

output "job_id" {
  description = "The full resource ID of the provisioned Cloud Run v2 Job."
  value       = google_cloud_run_v2_job.sync_job.id
}

output "service_account_email" {
  description = "Deterministic service account email running the ingestion Cloud Run Job."
  value       = local.sa_email
}

output "scheduler_job_name" {
  description = "The name of the Cloud Scheduler job triggering ingestion."
  value       = google_cloud_scheduler_job.sync_schedule.name
}
