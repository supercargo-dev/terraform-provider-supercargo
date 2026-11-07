resource "google_monitoring_dashboard" "control_plane_sre" {
  count   = var.enable_dashboards ? 1 : 0
  project = var.project_id

  dashboard_json = jsonencode({
    displayName = "Supercargo - Control Plane & SRE Infrastructure (${var.environment})${local.name_suffix_display}"
    mosaicLayout = {
      columns = 12
      tiles = [
        # --- Section 1: Cloud Run Golden Signals ---
        {
          xPos   = 0
          yPos   = 0
          width  = 12
          height = 1
          widget = {
            title = "Cloud Run Golden Signals"
            text = {
              content = "### Section 1: Cloud Run Golden Signals (Request Volume, Error Rates, Latency)"
              format  = "MARKDOWN"
            }
          }
        },
        {
          xPos   = 0
          yPos   = 1
          width  = 4
          height = 3
          widget = {
            title = "Total Control Plane Request Rate"
            scorecard = {
              timeSeriesQuery = {
                timeSeriesFilter = {
                  filter = "resource.type=\"cloud_run_revision\" AND metric.type=\"run.googleapis.com/request_count\""
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
          xPos   = 4
          yPos   = 1
          width  = 4
          height = 3
          widget = {
            title = "HTTP 5xx Error Rate by Service"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloud_run_revision\" AND metric.type=\"run.googleapis.com/request_count\" AND metric.label.response_code_class=\"5xx\""
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
          xPos   = 8
          yPos   = 1
          width  = 4
          height = 3
          widget = {
            title = "Request Latency P99 by Service"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloud_run_revision\" AND metric.type=\"run.googleapis.com/request_latencies\""
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

        # --- Section 2: Container Resource Saturation ---
        {
          xPos   = 0
          yPos   = 4
          width  = 12
          height = 1
          widget = {
            title = "Container Resource Saturation"
            text = {
              content = "### Section 2: Container Resource Saturation (Memory, CPU, Active Container Ceiling)"
              format  = "MARKDOWN"
            }
          }
        },
        {
          xPos   = 0
          yPos   = 5
          width  = 4
          height = 3
          widget = {
            title = "Container Memory Utilization"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloud_run_revision\" AND metric.type=\"run.googleapis.com/container/memory/utilizations\""
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
          xPos   = 4
          yPos   = 5
          width  = 4
          height = 3
          widget = {
            title = "Container CPU Utilization"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloud_run_revision\" AND metric.type=\"run.googleapis.com/container/cpu/utilizations\""
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
          xPos   = 8
          yPos   = 5
          width  = 4
          height = 3
          widget = {
            title = "Active Container Instances"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloud_run_revision\" AND metric.type=\"run.googleapis.com/container/instance_count\" AND metric.label.state=\"active\""
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

        # --- Section 3: SLO Burn Rates ---
        {
          xPos   = 0
          yPos   = 8
          width  = 12
          height = 1
          widget = {
            title = "SLO Burn Rates"
            text = {
              content = "### Section 3: SLO Burn Rates (Hub & Vault Error Budget Consumption)"
              format  = "MARKDOWN"
            }
          }
        },
        {
          xPos   = 0
          yPos   = 9
          width  = 6
          height = 3
          widget = {
            title = "Hub Availability & Latency Burn Rates"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "select_slo_burn_rate(\"${local.hub_slo_prefix}/hub-availability\", \"3600s\")"
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
          width  = 6
          height = 3
          widget = {
            title = "Vault Availability & Latency Burn Rates"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "select_slo_burn_rate(\"${local.vault_slo_prefix}/vault-availability\", \"3600s\")"
                    }
                  }
                  plotType = "LINE"
                }
              ]
            }
          }
        },

        # --- Section 4: Pub/Sub & BigQuery Pipeline Health ---
        {
          xPos   = 0
          yPos   = 12
          width  = 12
          height = 1
          widget = {
            title = "Pub/Sub & BigQuery Pipeline Health"
            text = {
              content = "### Section 4: Pub/Sub & BigQuery Pipeline Health (Audit Sink #300, Health Mesh, Eventarc Delivery)"
              format  = "MARKDOWN"
            }
          }
        },
        {
          xPos   = 0
          yPos   = 13
          width  = 4
          height = 3
          widget = {
            title = "Core Subscriptions Oldest Unacked Message Age"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"pubsub_subscription\" AND metric.type=\"pubsub.googleapis.com/subscription/oldest_unacked_message_age\""
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
          xPos   = 4
          yPos   = 13
          width  = 4
          height = 3
          widget = {
            title = "Dataform DLQ Undelivered Messages"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"pubsub_subscription\" AND metric.type=\"pubsub.googleapis.com/subscription/num_undelivered_messages\" AND resource.labels.subscription_id=\"${var.dataform_events_dlq_subscription_name}\""
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
          xPos   = 8
          yPos   = 13
          width  = 4
          height = 3
          widget = {
            title = "Eventarc Shovel Trigger Failed Delivery Attempts"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"eventarc_trigger\" AND resource.labels.trigger_id=\"${var.shovel_trigger_name}\" AND metric.type=\"eventarc.googleapis.com/delivery_attempt_count\" AND metric.label.response_code!=\"OK\""
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

        # --- Section 5: Batch Ingestion Jobs & Cloud Schedulers ---
        {
          xPos   = 0
          yPos   = 16
          width  = 12
          height = 1
          widget = {
            title = "Batch Ingestion Jobs & Cloud Schedulers"
            text = {
              content = "### Section 5: Batch Ingestion Jobs & Cloud Schedulers (Analytics Runner, GitHub Sync, Schedulers)"
              format  = "MARKDOWN"
            }
          }
        },
        {
          xPos   = 0
          yPos   = 17
          width  = 6
          height = 3
          widget = {
            title = "Cloud Run Job Completed Executions"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloud_run_job\" AND metric.type=\"run.googleapis.com/job/completed_execution_count\""
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
        {
          xPos   = 6
          yPos   = 17
          width  = 6
          height = 3
          widget = {
            title = "Cloud Scheduler Job Dispatch Attempts"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloud_scheduler_job\" AND metric.type=\"cloudscheduler.googleapis.com/job/attempt_count\""
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

        # --- Section 6: Meta-Telemetry Lakehouse Streaming Health ---
        {
          xPos   = 0
          yPos   = 20
          width  = 12
          height = 1
          widget = {
            title = "Meta-Telemetry Lakehouse Streaming Health"
            text = {
              content = "### Section 6: Meta-Telemetry Lakehouse Streaming Health (7 Operational BQ Subscriptions & DLQ)"
              format  = "MARKDOWN"
            }
          }
        },
        {
          xPos   = 0
          yPos   = 21
          width  = 6
          height = 3
          widget = {
            title = "Operational Telemetry BQ Subscriptions Oldest Unacked Message Age"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"pubsub_subscription\" AND metric.type=\"pubsub.googleapis.com/subscription/oldest_unacked_message_age\""
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
          yPos   = 21
          width  = 6
          height = 3
          widget = {
            title = "Platform Telemetry DLQ Undelivered Messages"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"pubsub_subscription\" AND resource.labels.subscription_id=\"${var.telemetry_dlq_subscription_name}\" AND metric.type=\"pubsub.googleapis.com/subscription/num_undelivered_messages\""
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
        }
      ]
    }
  })
}

resource "google_monitoring_dashboard" "governance_privacy" {
  count   = var.enable_dashboards ? 1 : 0
  project = var.project_id

  dashboard_json = jsonencode({
    displayName = "Supercargo - Governance & Privacy Plane (${var.environment})${local.name_suffix_display}"
    mosaicLayout = {
      columns = 12
      tiles = [
        # --- Section 1: Hub Contract Registry Operations ---
        {
          xPos   = 0
          yPos   = 0
          width  = 12
          height = 1
          widget = {
            title = "Hub Contract Registry Operations"
            text = {
              content = "### Section 1: Hub Contract Registry Operations (RPC Invocations, Schema Queries, Deletions)"
              format  = "MARKDOWN"
            }
          }
        },
        {
          xPos   = 0
          yPos   = 1
          width  = 6
          height = 3
          widget = {
            title = "Hub Request Volume Rate"
            scorecard = {
              timeSeriesQuery = {
                timeSeriesFilter = {
                  filter = "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${var.hub_service_name}\" AND metric.type=\"run.googleapis.com/request_count\""
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
          width  = 6
          height = 3
          widget = {
            title = "Hub Request Latencies P99 & P95"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${var.hub_service_name}\" AND metric.type=\"run.googleapis.com/request_latencies\""
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

        # --- Section 2: Firestore Outbox CDC & Metadata Shovel Delivery ---
        {
          xPos   = 0
          yPos   = 4
          width  = 12
          height = 1
          widget = {
            title = "Firestore Outbox CDC & Metadata Shovel Delivery"
            text = {
              content = "### Section 2: Firestore Outbox CDC & Metadata Shovel Delivery (Write Contention, Quota, Eventarc)"
              format  = "MARKDOWN"
            }
          }
        },
        {
          xPos   = 0
          yPos   = 5
          width  = 4
          height = 3
          widget = {
            title = "Firestore Operations on (default)"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"firestore_database\" AND resource.labels.database_id=\"${var.firestore_database_id}\" AND metric.type=\"firestore.googleapis.com/api/request_count\""
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
          xPos   = 4
          yPos   = 5
          width  = 4
          height = 3
          widget = {
            title = "Metadata Shovel Ingestion Rate"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${var.shovel_service_name}\" AND metric.type=\"run.googleapis.com/request_count\""
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
          xPos   = 8
          yPos   = 5
          width  = 4
          height = 3
          widget = {
            title = "Outbox Audit BigQuery Subscription Lag"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"pubsub_subscription\" AND metric.type=\"pubsub.googleapis.com/subscription/oldest_unacked_message_age\" AND resource.labels.subscription_id=\"${local.raw_core_subscriptions.outbox_audit_bq.name}\""
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

        # --- Section 3: Vault Cryptographic Operations & Security ---
        {
          xPos   = 0
          yPos   = 8
          width  = 12
          height = 1
          widget = {
            title = "Vault Cryptographic Operations & Security"
            text = {
              content = "### Section 3: Vault Cryptographic Operations & Security (Tokenization Rate, P99 Latency, 403 Spikes)"
              format  = "MARKDOWN"
            }
          }
        },
        {
          xPos   = 0
          yPos   = 9
          width  = 4
          height = 3
          widget = {
            title = "Vault Tokenization Rate"
            scorecard = {
              timeSeriesQuery = {
                timeSeriesFilter = {
                  filter = "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${var.vault_service_name}\" AND metric.type=\"run.googleapis.com/request_count\""
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
          xPos   = 4
          yPos   = 9
          width  = 4
          height = 3
          widget = {
            title = "Cloud KMS Crypto Request Volume"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloudkms_cryptokey\" AND metric.type=\"cloudkms.googleapis.com/crypto_requests\""
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
          xPos   = 8
          yPos   = 9
          width  = 4
          height = 3
          widget = {
            title = "Vault HTTP 403 Forbidden Rejections"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${var.vault_service_name}\" AND metric.type=\"run.googleapis.com/request_count\" AND metric.label.response_code=\"403\""
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

        # --- Section 4: Companion Service Interactions ---
        {
          xPos   = 0
          yPos   = 12
          width  = 12
          height = 1
          widget = {
            title = "Companion Service Interactions"
            text = {
              content = "### Section 4: Companion Service Interactions (Supercargo MCP & Console Direct IAP)"
              format  = "MARKDOWN"
            }
          }
        },
        {
          xPos   = 0
          yPos   = 13
          width  = 6
          height = 3
          widget = {
            title = "MCP Companion Service Requests"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${var.mcp_service_name}\" AND metric.type=\"run.googleapis.com/request_count\""
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
          yPos   = 13
          width  = 6
          height = 3
          widget = {
            title = "Console Companion Service Requests"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"${var.console_service_name}\" AND metric.type=\"run.googleapis.com/request_count\""
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

        # --- Section 5: Dataplex Taxonomy & Dataform Reconciliation State ---
        {
          xPos   = 0
          yPos   = 16
          width  = 12
          height = 1
          widget = {
            title = "Dataplex Taxonomy & Dataform Reconciliation State"
            text = {
              content = "### Section 5: Dataplex Taxonomy & Dataform Reconciliation State"
              format  = "MARKDOWN"
            }
          }
        },
        {
          xPos   = 0
          yPos   = 17
          width  = 6
          height = 3
          widget = {
            title = "Dataform Events Push Subscription Age"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"pubsub_subscription\" AND metric.type=\"pubsub.googleapis.com/subscription/oldest_unacked_message_age\" AND resource.labels.subscription_id=\"${local.raw_core_subscriptions.dataform_events_push.name}\""
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
          yPos   = 17
          width  = 6
          height = 3
          widget = {
            title = "Dataform Events DLQ Undelivered Messages"
            xyChart = {
              dataSets = [
                {
                  timeSeriesQuery = {
                    timeSeriesFilter = {
                      filter = "resource.type=\"pubsub_subscription\" AND metric.type=\"pubsub.googleapis.com/subscription/num_undelivered_messages\" AND resource.labels.subscription_id=\"${var.dataform_events_dlq_subscription_name}\""
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
        }
      ]
    }
  })
}
