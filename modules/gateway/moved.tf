moved {
  from = google_service_account.guardian
  to   = google_service_account.gateway
}

moved {
  from = google_cloud_run_v2_service.guardian
  to   = google_cloud_run_v2_service.gateway
}

moved {
  from = google_pubsub_topic_iam_member.guardian_publisher_clean
  to   = google_pubsub_topic_iam_member.gateway_publisher_clean
}

moved {
  from = google_pubsub_topic_iam_member.guardian_publisher_dlq
  to   = google_pubsub_topic_iam_member.gateway_publisher_dlq
}
