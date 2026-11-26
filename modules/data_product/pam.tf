locals {
  pam_eligible_outputs = {
    for k, v in try(data.supercargo_data_product.this.pam_entitlements, {}) : k => v
    if try(v.requires_pam, false)
  }

  master_kms_key_resource_name = var.master_key_uri != "" ? trimprefix(var.master_key_uri, "gcp-kms://") : ""

  conditional_consumer_table_bindings_list = flatten([
    for grant_key, grant in var.conditional_consumer_grants : [
      for output_key, out in local.public_output_views : [
        for member in grant.members : {
          key        = "${grant_key}:${output_key}:${trimspace(member)}"
          grant_key  = grant_key
          output_key = output_key
          member     = trimspace(member)
          role       = coalesce(grant.role, "roles/bigquery.dataViewer")
          expiration = grant.expiration
        }
      ]
    ]
  ])

  conditional_consumer_table_bindings = {
    for binding in local.conditional_consumer_table_bindings_list :
    binding.key => binding
  }
}

resource "google_privileged_access_manager_entitlement" "port_jit" {
  for_each = var.enable_pam_entitlements && length(var.pam_eligible_consumers) > 0 ? local.pam_eligible_outputs : {}

  parent               = "projects/${var.project_id}"
  location             = var.pam_location
  entitlement_id       = each.value.entitlement_id
  max_request_duration = var.pam_max_request_duration

  requester_justification_config {
    unstructured {}
  }

  eligible_users {
    principals = var.pam_eligible_consumers
  }

  privileged_access {
    gcp_iam_access {
      resource_type = "cloudresourcemanager.googleapis.com/Project"
      resource      = "//cloudresourcemanager.googleapis.com/projects/${var.project_id}"

      role_bindings {
        role                 = "roles/bigquery.dataViewer"
        condition_expression = "resource.name.startsWith(\"projects/${var.project_id}/datasets/${google_bigquery_dataset.public.dataset_id}\")"
      }

      dynamic "role_bindings" {
        for_each = var.enable_reidentified_kms_pam && each.value.requires_kms_decrypt && local.master_kms_key_resource_name != "" ? [1] : []
        content {
          role                 = "roles/cloudkms.cryptoKeyDecrypter"
          condition_expression = "resource.name == \"${local.master_kms_key_resource_name}\""
        }
      }
    }
  }

  approval_workflow {
    manual_approvals {
      require_approver_justification = true
      steps {
        approvals_needed = 1
        approvers {
          principals = var.pam_approvers
        }
      }
    }
  }

  lifecycle {
    precondition {
      condition     = length(var.pam_approvers) > 0
      error_message = "var.pam_approvers must contain at least one IAM principal (user:, group:, serviceAccount:, domain:) when enable_pam_entitlements is true and pam_eligible_consumers is non-empty."
    }
  }
}

resource "google_bigquery_table_iam_member" "conditional_consumers" {
  for_each   = local.conditional_consumer_table_bindings
  project    = var.project_id
  dataset_id = google_bigquery_dataset.public.dataset_id
  table_id   = google_bigquery_table.outputs[each.value.output_key].table_id
  role       = each.value.role
  member     = each.value.member

  condition {
    title       = "supercargo-jit-ttl-${each.value.grant_key}"
    description = "Supercargo time-bounded consumer grant (${each.value.grant_key}) expiring at ${each.value.expiration}"
    expression  = "request.time < timestamp(\"${each.value.expiration}\")"
  }
}
