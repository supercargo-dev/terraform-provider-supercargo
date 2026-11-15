locals {
  manifest_content     = yamldecode(file(var.manifest_file))
  product_id           = length(split(":", data.supercargo_data_product.this.urn)) > 1 ? element(split(":", data.supercargo_data_product.this.urn), length(split(":", data.supercargo_data_product.this.urn)) - 1) : data.supercargo_data_product.this.urn
  manifest_input_ports = try(local.manifest_content.input_ports, null) != null ? local.manifest_content.input_ports : []

  raw_var_contracts = [
    for k, v in var.contracts : {
      key    = length(split(":", k)) > 1 ? element(split(":", k), length(split(":", k)) - 1) : k
      id     = k
      schema = v.schema_json
    }
  ]
  grouped_var_contracts = {
    for c in local.raw_var_contracts : c.key => c...
  }
  var_gateway_contracts = {
    for k, items in local.grouped_var_contracts : k => {
      id     = items[0].id
      schema = try(coalesce([for i in items : i.schema if i.schema != null]...), null)
    }
  }
  gateway_contracts = length(var.contracts) > 0 ? local.var_gateway_contracts : {
    for k, c in data.supercargo_data_product.this.contracts : (
      length(split(":", k)) > 1 ? element(split(":", k), length(split(":", k)) - 1) : k
      ) => {
      id     = c.id
      schema = try(c.schema_json, lookup(c, "schema", null))
    }
  }

  public_dataset_id   = var.bigquery_dataset_id != "" ? var.bigquery_dataset_id : replace(local.product_id, "-", "_")
  internal_dataset_id = var.internal_bigquery_dataset_id != "" ? var.internal_bigquery_dataset_id : "${local.public_dataset_id}_internal"

  public_output_views = {
    for k, v in try(data.supercargo_data_product.this.outputs, {}) : k => v
    if lookup(v, "materialization", "view") == "view"
  }
  internal_input_tables = try(data.supercargo_data_product.this.inputs, {})

  # Deterministic runner identities bypassing child module outputs
  gateway_sa_id    = var.service_account_id != "" ? var.service_account_id : "gateway-${local.product_id}${var.name_suffix != "" ? "-${var.name_suffix}" : ""}"
  gateway_sa_email = "${local.gateway_sa_id}@${var.project_id}.iam.gserviceaccount.com"

  all_runner_emails = compact(concat(
    [local.gateway_sa_email],
    length(local.dlt_input_ports) > 0 ? [local.dlt_runner_sa_email] : [],
    var.runner_service_accounts
  ))

  all_runner_members = toset([
    for sa in local.all_runner_emails :
    startswith(trimspace(sa), "serviceAccount:") ? trimspace(sa) : "serviceAccount:${trimspace(sa)}"
  ])

  all_consumer_members = toset([
    for c in var.authorized_consumers : trimspace(c)
  ])
}

data "supercargo_data_product" "this" {
  manifest_file = var.manifest_file
  project       = var.project_id
  lock_dir      = var.lock_dir
}

module "gateway" {
  source = "../gateway"

  project_id                          = var.project_id
  region                              = var.region
  product_id                          = local.product_id
  image                               = var.image
  log_level                           = var.log_level
  container_memory                    = var.container_memory
  container_cpu                       = var.container_cpu
  pubsub_max_delivery_attempts        = var.pubsub_max_delivery_attempts
  pubsub_minimum_backoff              = var.pubsub_minimum_backoff
  pubsub_maximum_backoff              = var.pubsub_maximum_backoff
  pubsub_message_retention_duration   = var.pubsub_message_retention_duration
  pubsub_expiration_policy_ttl        = var.pubsub_expiration_policy_ttl
  hub_address                         = var.hub_address
  kms_address                         = var.kms_address
  master_key_uri                      = var.master_key_uri
  hub_oidc_audience                   = var.hub_oidc_audience
  vault_oidc_audience                 = var.vault_oidc_audience
  bigquery_dataset_id                 = local.internal_dataset_id
  auth_enforce                        = var.auth_enforce
  force_deploy_trigger                = var.force_deploy_trigger
  custom_audiences                    = var.custom_audiences
  gateway_audience                    = var.gateway_audience
  bigquery_deletion_protection        = var.bigquery_deletion_protection
  authorized_invokers                 = var.authorized_invokers
  authorized_invoker_service_accounts = var.authorized_invoker_service_accounts
  enable_dlq_alerts                   = var.enable_dlq_alerts
  dlq_alert_threshold                 = var.dlq_alert_threshold
  dlq_unacked_message_age_seconds     = var.dlq_unacked_message_age_seconds
  dlq_runbook_url                     = var.dlq_runbook_url
  alert_slack_channel                 = var.alert_slack_channel
  alert_email_address                 = var.alert_email_address
  alert_pagerduty_service_key         = var.alert_pagerduty_service_key
  alert_notification_channels         = var.alert_notification_channels
  name_suffix                         = var.name_suffix
  service_account_id                  = var.service_account_id
  max_instances                       = var.max_instances

  sla_tier                              = var.sla_tier
  enable_infrastructure_alerts          = var.enable_infrastructure_alerts
  enable_monitoring_dashboard           = var.enable_monitoring_dashboard
  additional_sre_notification_channels  = var.additional_sre_notification_channels
  unacked_message_age_threshold_seconds = var.unacked_message_age_threshold_seconds

  contracts = local.gateway_contracts
}

resource "google_bigquery_dataset" "internal" {
  project                    = var.project_id
  dataset_id                 = local.internal_dataset_id
  location                   = var.region
  delete_contents_on_destroy = !var.bigquery_deletion_protection
}

resource "google_bigquery_dataset" "public" {
  project                    = var.project_id
  dataset_id                 = local.public_dataset_id
  location                   = var.region
  delete_contents_on_destroy = !var.bigquery_deletion_protection
}

resource "google_bigquery_dataset_access" "internal_views_delegation" {
  project    = var.project_id
  dataset_id = google_bigquery_dataset.internal.dataset_id

  dataset {
    dataset {
      project_id = var.project_id
      dataset_id = google_bigquery_dataset.public.dataset_id
    }
    target_types = ["VIEWS"]
  }
}

resource "google_bigquery_table" "inputs" {
  for_each            = local.internal_input_tables
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.internal.dataset_id
  table_id            = try(each.value.table_name, each.value.record_name, each.value.name)
  deletion_protection = var.bigquery_deletion_protection
  schema              = each.value.schema_json
}

resource "google_bigquery_table" "outputs" {
  for_each            = local.public_output_views
  project             = var.project_id
  dataset_id          = google_bigquery_dataset.public.dataset_id
  table_id            = each.value.name
  deletion_protection = var.bigquery_deletion_protection

  view {
    query          = each.value.view_query
    use_legacy_sql = false
  }

  depends_on = [
    google_bigquery_dataset_access.internal_views_delegation,
    google_bigquery_table.inputs
  ]
}

resource "google_bigquery_dataset_iam_member" "runners_data_editor" {
  for_each   = local.all_runner_members
  project    = var.project_id
  dataset_id = google_bigquery_dataset.internal.dataset_id
  role       = "roles/bigquery.dataEditor"
  member     = each.value
}

resource "google_bigquery_dataset_iam_member" "consumers_data_viewer" {
  for_each   = local.all_consumer_members
  project    = var.project_id
  dataset_id = google_bigquery_dataset.public.dataset_id
  role       = "roles/bigquery.dataViewer"
  member     = each.value
}

