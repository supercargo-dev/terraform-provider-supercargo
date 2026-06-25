moved {
  from = google_service_account.kms_sa
  to   = google_service_account.vault_sa
}

moved {
  from = google_kms_crypto_key_iam_member.kms_sa_binding
  to   = google_kms_crypto_key_iam_member.vault_sa_binding
}

moved {
  from = google_project_iam_member.kms_firestore_user
  to   = google_project_iam_member.vault_firestore_user
}

moved {
  from = google_pubsub_topic_iam_member.kms_pubsub_publisher
  to   = google_pubsub_topic_iam_member.vault_pubsub_publisher
}

moved {
  from = google_cloud_run_v2_service.kms
  to   = google_cloud_run_v2_service.vault
}
