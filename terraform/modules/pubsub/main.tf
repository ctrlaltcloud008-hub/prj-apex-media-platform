locals {
  default_lables = merge(var.labels, {
    environment = var.environment
    managed_by  = "terraform"
  })

  topics = [
    "video.gcs.finalized",
    "video.validated",
    "video.transcoding.complete",
    "video.fanout.thumbnail",
    "video.fanout.transcription",
    "video.fanout.moderation",
    "video.saga.thumbnail",
    "video.saga.transcription",
    "video.saga.moderation",
    "video.status.update",
    "video.dlq",
  ]

  subscriptions = {
    "sub-ingestion" = {
      topic = "video.gcs.finalized"
      type  = "pull"
    }
    "sub-transcode-orch" = {
      topic = "video.validated"
      type  = "pull"
    }
    "sub-transcode-callback" = {
      topic = "video.transcoding.complete"
      type  = "push"
    }
    "sub-thumbnail-orch" = {
      topic = "video.fanout.thumbnail"
      type  = "pull"
    }
    "sub-transcription-orch" = {
      topic = "video.fanout.transcription"
      type  = "pull"
    }
    "sub-moderation" = {
      topic = "video.fanout.moderation"
      type  = "push"
    }
    "sub-saga-thumbnail" = {
      topic = "video.saga.thumbnail"
      type  = "push"
    }
    "sub-saga-transcription" = {
      topic = "video.saga.transcription"
      type  = "push"
    }
    "sub-saga-moderation" = {
      topic = "video.saga.moderation"
      type  = "push"
    }
    "sub-notification" = {
      topic = "video.status.update"
      type  = "push"
    }
  }

  dlq_subscription = "sub-dlq-processor"

  placeholder_push_endpoint = "https://example.com/push-endpoint"
}

resource "google_pubsub_topic" "topics" {
  for_each = toset(local.topics)

  project = var.project_id
  name    = each.value

  message_retention_duration = var.message_retention_duration
  labels                     = local.default_lables
}

resource "google_pubsub_subscription" "subscriptions" {
  for_each = local.subscriptions

  project = var.project_id
  name    = each.key
  topic   = google_pubsub_topic.topics[each.value.topic].id

  ack_deadline_seconds       = var.ack_deadline_seconds
  message_retention_duration = var.message_retention_duration
  retain_acked_messages      = false

  dynamic "push_config" {
    for_each = each.value.type == "push" ? [1] : []
    content {
      push_endpoint = lookup(var.push_endpoints, each.key, local.placeholder_push_endpoint)

      attributes = {
        x-goog-version = "v1"
      }
    }
  }

  retry_policy {
    minimum_backoff = var.retry_minimum_backoff
    maximum_backoff = var.retry_maximum_backoff
  }

  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.topics["video.dlq"].id
    max_delivery_attempts = var.dlq_max_delivery_attempts
  }

  labels = local.default_lables
}

resource "google_pubsub_topic_iam_member" "gcs_publisher" {
  project = var.project_id
  topic   = google_pubsub_topic.topics["video.gcs.finalized"].name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${var.gcs_service_account_email}"
}
