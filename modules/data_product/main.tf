locals {
  manifest_content = yamldecode(file(var.manifest_file))
  product_urn      = try(local.manifest_content.meta.urn, "")
  product_id       = length(split(":", local.product_urn)) > 1 ? element(split(":", local.product_urn), length(split(":", local.product_urn)) - 1) : local.product_urn
  manifest_dir     = dirname(var.manifest_file)

  # Safely collect ports from manifest
  manifest_output_ports = try(local.manifest_content.output_ports, null) != null ? local.manifest_content.output_ports : []
  manifest_input_ports  = try(local.manifest_content.input_ports, null) != null ? local.manifest_content.input_ports : []
  manifest_all_ports    = concat(local.manifest_output_ports, local.manifest_input_ports)

  # Extract raw contract references
  raw_manifest_contracts = [
    for p in local.manifest_all_ports : {
      key = (
        length(split(":", try(p.contract.urn, p.name))) > 1
        ? element(split(":", try(p.contract.urn, p.name)), length(split(":", try(p.contract.urn, p.name))) - 1)
        : try(p.contract.urn, p.name)
      )
      id = try(p.contract.urn, p.name)
      schema = try(
        file("${local.manifest_dir}/schemas/${try(p.name, "")}.${try(p.contract.version, "")}.bigquery.json"),
        file("${local.manifest_dir}/schemas/${replace(try(p.contract.urn, p.name), ":", "_")}.bigquery.json"),
        file("${local.manifest_dir}/schemas/${try(p.name, "")}.bigquery.json"),
        file("${local.manifest_dir}/schemas/${length(split(":", try(p.contract.urn, p.name))) > 1 ? element(split(":", try(p.contract.urn, p.name)), length(split(":", try(p.contract.urn, p.name))) - 1) : try(p.contract.urn, p.name)}.${try(p.contract.version, "")}.bigquery.json"),
        file("${local.manifest_dir}/schemas/${length(split(":", try(p.contract.urn, p.name))) > 1 ? element(split(":", try(p.contract.urn, p.name)), length(split(":", try(p.contract.urn, p.name))) - 1) : try(p.contract.urn, p.name)}.bigquery.json"),
        file("${local.manifest_dir}/${try(p.name, "")}.bigquery.json"),
        null
      )
    }
    if try(p.contract, null) != null
  ]

  # Deduplicate by contract key safely using HCL grouping
  grouped_manifest_contracts = {
    for c in local.raw_manifest_contracts : c.key => c...
  }

  manifest_gateway_contracts = {
    for k, items in local.grouped_manifest_contracts : k => {
      id     = items[0].id
      schema = items[0].schema
    }
  }

  gateway_contracts = length(var.contracts) > 0 ? {
    for k, v in var.contracts : element(split(":", k), length(split(":", k)) - 1) => {
      id     = k
      schema = v.schema_json
    }
  } : local.manifest_gateway_contracts
}

resource "supercargo_data_product" "this" {
  manifest_file      = var.manifest_file
  project            = var.project_id
  partitioning_field = try(local.manifest_content.output_ports[0].physical.bigquery.partition_by, "")
}

resource "supercargo_contract_version" "this" {
  for_each = var.contracts

  urn          = each.key
  version      = each.value.version
  schema_json  = each.value.schema_json
  commit_sha   = each.value.commit_sha
  content_hash = each.value.content_hash
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

  contracts = local.gateway_contracts

  depends_on = [
    supercargo_data_product.this
  ]
}

