resource "google_monitoring_dashboard" "data_product" {
  count   = var.enable_monitoring_dashboard ? 1 : 0
  project = var.project_id

  dashboard_json = jsonencode({
    displayName = "Supercargo Data Product - ${var.product_id}${local.name_suffix_display}"
    mosaicLayout = {
      columns = 12
      tiles = [
        # Section 1: Contract Validation & Quarantine Health
        {
          xPos   = 0
          yPos   = 0
          width  = 12
          height = 1
          widget = {
            title = "Contract Validation & Quarantine Health"
            text = {
              content = "### Section 1: Contract Validation & Quarantine Health"
              format  = "MARKDOWN"
            }
          }
        },
        {
          xPos   = 0
          yPos   = 1
          width  = 3
          height = 3
          widget = {
            title = "Total Messages Ingested"
            scorecard = {
              timeSeriesQuery = {
                timeSeriesFilter = {
                  filter = "metric.type=\"custom.googleapis.com/supercargo/gateway/messages_total\" AND metric.label.product_id=\"${var.product_id}\""
                  aggregation = {
                    alignmentPeriod  = "60s"
                    perSeriesAligner = "ALIGN_RATE"
                  }
                }
              }
            }
          }
        },
        {
          xPos   = 3
          yPos   = 1
          width  = 3
          height = 3
          widget = {
            title = "Validation Errors"
            scorecard = {
              timeSeriesQuery = {
                timeSeriesFilter = {
                  filter = "metric.type=\"custom.googleapis.com/supercargo/gateway/validation_errors_total\" AND metric.label.product_id=\"${var.product_id}\""
                  aggregation = {
                    alignmentPeriod  = "60s"
                    perSeriesAligner = "ALIGN_RATE"
                  }
                }
              }
            }
          }
        },
        {
          xPos   = 6
          yPos   = 1
          width  = 3
          height = 3
          widget = {
            title = "Validation Duration (Latency)"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "metric.type=\"custom.googleapis.com/supercargo/gateway/validation_duration_seconds\" AND metric.label.product_id=\"${var.product_id}\""
                      aggregation = {
                        alignmentPeriod  = "60s"
                        perSeriesAligner = "ALIGN_PERCENTILE_95"
                      }
                    }
                  }
                  plotType = "LINE"
                }
              ]
            }
          }
        },
        {
          xPos   = 9
          yPos   = 1
          width  = 3
          height = 3
          widget = {
            title = "DLQ Oldest Unacked Message Age"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"pubsub_subscription\" AND resource.labels.subscription_id=\"${google_pubsub_subscription.dlq_sub.name}\" AND metric.type=\"pubsub.googleapis.com/subscription/oldest_unacked_message_age\""
                      aggregation = {
                        alignmentPeriod  = "60s"
                        perSeriesAligner = "ALIGN_MAX"
                      }
                    }
                  }
                  plotType = "LINE"
                }
              ]
            }
          }
        },

        # Section 2: Gateway Cloud Run Golden Signals & Scaling
        {
          xPos   = 0
          yPos   = 4
          width  = 12
          height = 1
          widget = {
            title = "Gateway Cloud Run Golden Signals & Scaling"
            text = {
              content = "### Section 2: Gateway Cloud Run Golden Signals & Scaling"
              format  = "MARKDOWN"
            }
          }
        },
        {
          xPos   = 0
          yPos   = 5
          width  = 3
          height = 3
          widget = {
            title = "Request Count by Response Code Class"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${google_cloud_run_v2_service.gateway.name}\" AND metric.type=\"run.googleapis.com/request_count\""
                      aggregation = {
                        alignmentPeriod  = "60s"
                        perSeriesAligner = "ALIGN_RATE"
                        groupByFields    = ["metric.label.response_code_class"]
                      }
                    }
                  }
                  plotType = "LINE"
                }
              ]
            }
          }
        },
        {
          xPos   = 3
          yPos   = 5
          width  = 3
          height = 3
          widget = {
            title = "Request Latencies (P99)"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${google_cloud_run_v2_service.gateway.name}\" AND metric.type=\"run.googleapis.com/request_latencies\""
                      aggregation = {
                        alignmentPeriod  = "60s"
                        perSeriesAligner = "ALIGN_PERCENTILE_99"
                      }
                    }
                  }
                  plotType = "LINE"
                }
              ]
            }
          }
        },
        {
          xPos   = 6
          yPos   = 5
          width  = 3
          height = 3
          widget = {
            title = "CPU & Memory Utilization"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${google_cloud_run_v2_service.gateway.name}\" AND metric.type=\"run.googleapis.com/container/cpu/utilizations\""
                      aggregation = {
                        alignmentPeriod  = "60s"
                        perSeriesAligner = "ALIGN_PERCENTILE_99"
                      }
                    }
                  }
                  plotType = "LINE"
                },
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${google_cloud_run_v2_service.gateway.name}\" AND metric.type=\"run.googleapis.com/container/memory/utilizations\""
                      aggregation = {
                        alignmentPeriod  = "60s"
                        perSeriesAligner = "ALIGN_PERCENTILE_99"
                      }
                    }
                  }
                  plotType = "LINE"
                }
              ]
            }
          }
        },
        {
          xPos   = 9
          yPos   = 5
          width  = 3
          height = 3
          widget = {
            title = "Active Container Instances"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${google_cloud_run_v2_service.gateway.name}\" AND metric.type=\"run.googleapis.com/container/instance_count\" AND metric.label.state=\"active\""
                      aggregation = {
                        alignmentPeriod  = "60s"
                        perSeriesAligner = "ALIGN_MAX"
                      }
                    }
                  }
                  plotType = "LINE"
                }
              ]
            }
          }
        },

        # Section 3: Pub/Sub Ingress & BigQuery Sink Backlogs
        {
          xPos   = 0
          yPos   = 8
          width  = 12
          height = 1
          widget = {
            title = "Pub/Sub Ingress & BigQuery Sink Backlogs"
            text = {
              content = "### Section 3: Pub/Sub Ingress & BigQuery Sink Backlogs"
              format  = "MARKDOWN"
            }
          }
        },
        {
          xPos   = 0
          yPos   = 9
          width  = 3
          height = 3
          widget = {
            title = "Ingress Oldest Unacked Message Age"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"pubsub_subscription\" AND resource.labels.subscription_id=\"${google_pubsub_subscription.raw_push.name}\" AND metric.type=\"pubsub.googleapis.com/subscription/oldest_unacked_message_age\""
                      aggregation = {
                        alignmentPeriod  = "60s"
                        perSeriesAligner = "ALIGN_MAX"
                      }
                    }
                  }
                  plotType = "LINE"
                }
              ]
            }
          }
        },
        {
          xPos   = 3
          yPos   = 9
          width  = 3
          height = 3
          widget = {
            title = "Ingress Num Undelivered Messages"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"pubsub_subscription\" AND resource.labels.subscription_id=\"${google_pubsub_subscription.raw_push.name}\" AND metric.type=\"pubsub.googleapis.com/subscription/num_undelivered_messages\""
                      aggregation = {
                        alignmentPeriod  = "60s"
                        perSeriesAligner = "ALIGN_MAX"
                      }
                    }
                  }
                  plotType = "LINE"
                }
              ]
            }
          }
        },
        {
          xPos   = 6
          yPos   = 9
          width  = 3
          height = 3
          widget = {
            title = "Clean BigQuery Sinks Oldest Unacked Age"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"pubsub_subscription\" AND resource.labels.subscription_id=starts_with(\"clean-bq-${var.product_id}-\") AND metric.type=\"pubsub.googleapis.com/subscription/oldest_unacked_message_age\""
                      aggregation = {
                        alignmentPeriod  = "60s"
                        perSeriesAligner = "ALIGN_MAX"
                      }
                    }
                  }
                  plotType = "LINE"
                }
              ]
            }
          }
        },
        {
          xPos   = 9
          yPos   = 9
          width  = 3
          height = 3
          widget = {
            title = "Clean BigQuery Dead-Letter Message Count"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"pubsub_subscription\" AND resource.labels.subscription_id=starts_with(\"clean-bq-${var.product_id}-\") AND metric.type=\"pubsub.googleapis.com/subscription/dead_letter_message_count\""
                      aggregation = {
                        alignmentPeriod  = "60s"
                        perSeriesAligner = "ALIGN_DELTA"
                      }
                    }
                  }
                  plotType = "LINE"
                }
              ]
            }
          }
        },

        # Section 4: DLT Batch Jobs & Schedulers
        {
          xPos   = 0
          yPos   = 12
          width  = 12
          height = 1
          widget = {
            title = "DLT Batch Jobs & Schedulers"
            text = {
              content = "### Section 4: DLT Batch Jobs & Schedulers"
              format  = "MARKDOWN"
            }
          }
        },
        {
          xPos   = 0
          yPos   = 13
          width  = 6
          height = 4
          widget = {
            title = "DLT Cloud Run Job Completed Executions"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloud_run_job\" AND resource.labels.job_name=starts_with(\"dlt-${var.product_id}-\") AND metric.type=\"run.googleapis.com/job/completed_execution_count\""
                      aggregation = {
                        alignmentPeriod  = "60s"
                        perSeriesAligner = "ALIGN_DELTA"
                        groupByFields    = ["metric.label.result"]
                      }
                    }
                  }
                  plotType = "LINE"
                }
              ]
            }
          }
        },
        {
          xPos   = 6
          yPos   = 13
          width  = 6
          height = 4
          widget = {
            title = "DLT Cloud Scheduler Attempt Counts"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloud_scheduler_job\" AND resource.labels.job_id=starts_with(\"dlt-${var.product_id}-\") AND metric.type=\"cloudscheduler.googleapis.com/job/attempt_count\""
                      aggregation = {
                        alignmentPeriod  = "60s"
                        perSeriesAligner = "ALIGN_DELTA"
                        groupByFields    = ["metric.label.response_status"]
                      }
                    }
                  }
                  plotType = "LINE"
                }
              ]
            }
          }
        },

        # Section 5: BigQuery Landing Table Throughput
        {
          xPos   = 0
          yPos   = 17
          width  = 12
          height = 1
          widget = {
            title = "BigQuery Landing Table Throughput"
            text = {
              content = "### Section 5: BigQuery Landing Table Throughput"
              format  = "MARKDOWN"
            }
          }
        },
        {
          xPos   = 0
          yPos   = 18
          width  = 6
          height = 4
          widget = {
            title = "Clean BigQuery Sink Push Request Rate"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"pubsub_subscription\" AND resource.labels.subscription_id=starts_with(\"clean-bq-${var.product_id}-\") AND metric.type=\"pubsub.googleapis.com/subscription/push_request_count\""
                      aggregation = {
                        alignmentPeriod  = "60s"
                        perSeriesAligner = "ALIGN_RATE"
                      }
                    }
                  }
                  plotType = "LINE"
                }
              ]
            }
          }
        },
        {
          xPos   = 6
          yPos   = 18
          width  = 6
          height = 4
          widget = {
            title = "BigQuery Storage & Ingestion Throughput"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"bigquery_dataset\" AND metric.type=\"bigquery.googleapis.com/storage/uploaded_row_count\""
                      aggregation = {
                        alignmentPeriod  = "60s"
                        perSeriesAligner = "ALIGN_RATE"
                      }
                    }
                  }
                  plotType = "LINE"
                }
              ]
            }
          }
        }
      ]
    }
  })
}
