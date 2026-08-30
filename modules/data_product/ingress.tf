locals {
  raw_input_ports = try(local.manifest_content.input_ports, [])

  # Filter pubsub input ports
  pubsub_input_ports = {
    for port in local.raw_input_ports : port.name => port
    if can(port.source) && startswith(port.source, "pubsub://")
  }

  # Filter managed pubsub input ports (e.g. source == "pubsub://managed")
  managed_pubsub_ports = {
    for k, port in local.pubsub_input_ports : k => port
    if port.source == "pubsub://managed"
  }
}

data "google_project" "current" {
  project_id = var.project_id
}

// Managed Pub/Sub Topics for input ports specifying pubsub://managed
resource "google_pubsub_topic" "managed_input" {
  for_each = local.managed_pubsub_ports
  project  = var.project_id
  name     = "${var.project_id}-${each.key}-topic"
}

// Push subscriptions forwarding messages from input topics to the Gateway
resource "google_pubsub_subscription" "input_push" {
  for_each = local.pubsub_input_ports
  name     = "input-sub-${local.product_id}-${each.key}"
  topic = (
    each.value.source == "pubsub://managed"
    ? google_pubsub_topic.managed_input[each.key].id
    : (
      startswith(each.value.source, "pubsub://projects/")
      ? trimprefix(each.value.source, "pubsub://")
      : "projects/${var.project_id}/topics/${trimprefix(each.value.source, "pubsub://")}"
    )
  )
  project = var.project_id

  message_retention_duration = var.pubsub_message_retention_duration
  expiration_policy {
    ttl = ""
  }

  push_config {
    push_endpoint = can(each.value.contract.urn) ? "${module.gateway.service_url}/pubsub/push/${each.value.contract.urn}" : "${module.gateway.service_url}/pubsub/push/${each.key}"
    oidc_token {
      service_account_email = module.gateway.push_invoker_service_account_email
      audience              = var.gateway_audience
    }
  }

  dead_letter_policy {
    dead_letter_topic     = module.gateway.dlq_topic_id
    max_delivery_attempts = var.pubsub_max_delivery_attempts
  }

  retry_policy {
    minimum_backoff = var.pubsub_minimum_backoff
    maximum_backoff = var.pubsub_maximum_backoff
  }

  depends_on = [
    module.gateway
  ]
}

// IAM: Pub/Sub Service Agent needs permission to acknowledge messages on input push subscriptions for DLQ routing
resource "google_pubsub_subscription_iam_member" "pubsub_input_dlq_subscriber" {
  for_each     = local.pubsub_input_ports
  project      = var.project_id
  subscription = google_pubsub_subscription.input_push[each.key].name
  role         = "roles/pubsub.subscriber"
  member       = "serviceAccount:service-${data.google_project.current.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}
