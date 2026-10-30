locals {
  # AC-1: Filter DLT input ports directly from manifest input ports without redefining raw_input_ports
  dlt_input_ports = {
    for port in local.manifest_input_ports : port.name => port
    if(can(port.type) && port.type == "dlt") || (can(port.source) && startswith(port.source, "dlt://"))
  }

  # AC-1: BigQuery dataset fallback
  dlt_bigquery_dataset = var.bigquery_dataset_id != "" ? var.bigquery_dataset_id : replace(local.product_id, "-", "_")

  # AC-2: Collision-resistant Runner Service Account ID and Email
  candidate_runner_sa = var.dlt_runner_service_account_id != "" ? var.dlt_runner_service_account_id : "dlt-r-${local.product_id}${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  dlt_runner_sa_id    = length(local.candidate_runner_sa) <= 30 ? local.candidate_runner_sa : "${substr(regexreplace(substr(local.candidate_runner_sa, 0, 23), "-+$", ""), 0, 23)}-${substr(md5(local.candidate_runner_sa), 0, 6)}"
  dlt_runner_sa_email = "${local.dlt_runner_sa_id}@${var.project_id}.iam.gserviceaccount.com"

  # AC-2: Collision-resistant Scheduler Service Account ID and Email
  candidate_scheduler_sa = var.dlt_scheduler_service_account_id != "" ? var.dlt_scheduler_service_account_id : "dlt-s-${local.product_id}${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  dlt_scheduler_sa_id    = length(local.candidate_scheduler_sa) <= 30 ? local.candidate_scheduler_sa : "${substr(regexreplace(substr(local.candidate_scheduler_sa, 0, 23), "-+$", ""), 0, 23)}-${substr(md5(local.candidate_scheduler_sa), 0, 6)}"
  dlt_scheduler_sa_email = "${local.dlt_scheduler_sa_id}@${var.project_id}.iam.gserviceaccount.com"
}
