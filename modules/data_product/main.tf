resource "supercargo_data_product" "this" {
  manifest_file = var.manifest_file
  project       = var.project_id
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

  project_id                        = var.project_id
  region                            = var.region
  product_id                        = element(split(":", supercargo_data_product.this.urn), length(split(":", supercargo_data_product.this.urn)) - 1)
  image                             = var.image
  log_level                         = var.log_level
  container_memory                  = var.container_memory
  container_cpu                     = var.container_cpu
  pubsub_max_delivery_attempts      = var.pubsub_max_delivery_attempts
  pubsub_minimum_backoff            = var.pubsub_minimum_backoff
  pubsub_maximum_backoff            = var.pubsub_maximum_backoff
  pubsub_message_retention_duration = var.pubsub_message_retention_duration
  pubsub_expiration_policy_ttl      = var.pubsub_expiration_policy_ttl
  hub_address                       = var.hub_address
  kms_address                       = var.kms_address
  master_key_uri                    = var.master_key_uri
  hub_iap_client_id                 = var.hub_iap_client_id
  vault_iap_client_id               = var.vault_iap_client_id
  hub_oidc_audience                 = var.hub_oidc_audience
  vault_oidc_audience               = var.vault_oidc_audience
  bigquery_dataset_id               = var.bigquery_dataset_id
  auth_enforce                      = var.auth_enforce
  force_deploy_trigger              = var.force_deploy_trigger
  custom_audiences                  = var.custom_audiences
  gateway_audience                  = var.gateway_audience
  bigquery_deletion_protection      = var.bigquery_deletion_protection

  contracts = {
    for k, v in var.contracts : element(split(":", supercargo_contract_version.this[k].urn), length(split(":", supercargo_contract_version.this[k].urn)) - 1) => {
      id     = supercargo_contract_version.this[k].urn
      schema = v.schema_json
    }
  }

  depends_on = [
    supercargo_contract_version.this
  ]
}
