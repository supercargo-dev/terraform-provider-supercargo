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
      schema = c.schema
    }
  }
}

data "supercargo_data_product" "this" {
  manifest_file = var.manifest_file
  project       = var.project_id
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
  hub_iap_client_id                   = var.hub_iap_client_id
  vault_iap_client_id                 = var.vault_iap_client_id
  hub_oidc_audience                   = var.hub_oidc_audience
  vault_oidc_audience                 = var.vault_oidc_audience
  bigquery_dataset_id                 = var.bigquery_dataset_id
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

  contracts = local.gateway_contracts
}
