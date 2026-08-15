resource "google_bigquery_dataset" "security_vault" {
  dataset_id = "security_vault"
  project    = var.project_id
  location   = var.location

  labels = {
    env = var.env
  }
}

resource "google_bigquery_table" "lookup_table" {
  dataset_id = google_bigquery_dataset.security_vault.dataset_id
  table_id   = "lookup_table"
  project    = var.project_id

  deletion_protection = var.bigquery_deletion_protection

  time_partitioning {
    type  = "DAY"
    field = "created_at"
  }

  clustering = ["id_search_hash"]

  schema = <<EOF
[
  {
    "name": "token",
    "type": "STRING",
    "mode": "REQUIRED",
    "description": "The Sovereign Token (HMAC-SHA256 of PII)."
  },
  {
    "name": "ciphertext",
    "type": "BYTES",
    "mode": "REQUIRED",
    "description": "The KMS-encrypted cleartext PII."
  },
  {
    "name": "id_search_hash",
    "type": "STRING",
    "mode": "REQUIRED",
    "description": "The Blind Index (HMAC-SHA256 of Federated ID)."
  },
  {
    "name": "entity_urn",
    "type": "STRING",
    "mode": "REQUIRED",
    "description": "The Federated URN of the entity."
  },
  {
    "name": "created_at",
    "type": "TIMESTAMP",
    "mode": "REQUIRED",
    "description": "The time the mapping was created."
  }
]
EOF
}

resource "google_iam_deny_policy" "restrict_bq_access" {
  count        = var.authorized_view_sa_email != "" ? 1 : 0
  provider     = google-beta
  parent       = "cloudresourcemanager.googleapis.com/projects/${var.project_id}"
  name         = "deny-bq-data-viewer-${var.env}"
  display_name = "Deny BigQuery Data Viewer on Security Vault"

  rules {
    description = "Deny bigquery.dataViewer to all users except authorized service accounts"
    deny_rule {
      denied_principals = ["principalSet://goog.global/attribute.value/allAuthenticatedUsers"]
      exception_principals = [
        "principal://iam.googleapis.com/projects/-/serviceAccounts/${var.authorized_view_sa_email}"
      ]
      denied_permissions = ["bigquery.googleapis.com/tables.getData"]
    }
  }
}

resource "google_bigquery_connection" "kms_connection" {
  connection_id = "kms-conn-${var.env}"
  project       = var.project_id
  location      = var.location
  friendly_name = "KMS Cloud Resource Connection for AEAD"
  cloud_resource {}
}

resource "google_kms_crypto_key_iam_member" "kms_decrypter" {
  crypto_key_id = var.sovereign_kms_key_id
  role          = "roles/cloudkms.cryptoKeyDecrypter"
  member        = "serviceAccount:${google_bigquery_connection.kms_connection.cloud_resource[0].service_account_id}"
}

resource "google_project_service" "bq_datatransfer" {
  project            = var.project_id
  service            = "bigquerydatatransfer.googleapis.com"
  disable_on_destroy = false
}

resource "google_bigquery_table" "rtbf_shred_queue" {
  dataset_id = google_bigquery_dataset.security_vault.dataset_id
  table_id   = "rtbf_shred_queue"
  project    = var.project_id

  deletion_protection = var.bigquery_deletion_protection

  schema = <<EOF
[
  { "name": "id_search_hash", "type": "STRING", "mode": "REQUIRED" },
  { "name": "entity_urn", "type": "STRING", "mode": "NULLABLE" },
  { "name": "processed", "type": "BOOLEAN", "mode": "NULLABLE" },
  { "name": "processed_at", "type": "TIMESTAMP", "mode": "NULLABLE" }
]
EOF
}

resource "google_pubsub_subscription" "vault_delete_to_bq" {
  name    = "vault-delete-to-bq-queue"
  topic   = var.platform_topic_id
  project = var.project_id

  bigquery_config {
    table               = "${var.project_id}.${google_bigquery_dataset.security_vault.dataset_id}.${google_bigquery_table.rtbf_shred_queue.table_id}"
    use_table_schema    = true
    use_topic_schema    = false
    write_metadata      = false
    drop_unknown_fields = true
  }

  filter = "attributes.urn = \"urn:supercargo:vault:contract:mapping-created\" AND attributes.action = \"DELETE\""
}

resource "google_bigquery_data_transfer_config" "scheduled_shredder" {
  display_name           = "RTBF Shredder (Scheduled)"
  location               = var.location
  project                = var.project_id
  data_source_id         = "scheduled_query"
  schedule               = "every 15 minutes"
  destination_dataset_id = google_bigquery_dataset.security_vault.dataset_id
  params = {
    query = <<EOF
BEGIN TRANSACTION;
DELETE FROM `${var.project_id}.${google_bigquery_dataset.security_vault.dataset_id}.lookup_table`
WHERE id_search_hash IN (
  SELECT id_search_hash
  FROM `${var.project_id}.${google_bigquery_dataset.security_vault.dataset_id}.rtbf_shred_queue`
  WHERE processed IS NULL OR processed = FALSE
);
UPDATE `${var.project_id}.${google_bigquery_dataset.security_vault.dataset_id}.rtbf_shred_queue`
SET processed = TRUE, processed_at = CURRENT_TIMESTAMP()
WHERE processed IS NULL OR processed = FALSE;
COMMIT TRANSACTION;
EOF
  }

  depends_on = [google_project_service.bq_datatransfer]

  timeouts {
    create = "2m"
    update = "2m"
    delete = "2m"
  }
}

data "google_project" "project" {
  project_id = var.project_id
}

resource "google_bigquery_dataset_iam_member" "pubsub_bq_editor" {
  dataset_id = google_bigquery_dataset.security_vault.dataset_id
  project    = var.project_id
  role       = "roles/bigquery.dataEditor"
  member     = "serviceAccount:service-${data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

resource "google_bigquery_dataset_iam_member" "pubsub_bq_metadata" {
  dataset_id = google_bigquery_dataset.security_vault.dataset_id
  project    = var.project_id
  role       = "roles/bigquery.metadataViewer"
  member     = "serviceAccount:service-${data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}
