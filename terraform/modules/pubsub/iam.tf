resource "google_project_service_identity" "pubsub" {
  provider = google-beta
  project  = var.project_id
  service  = "pubsub.googleapis.com"
}

locals {
  topic_publishers = {
    "video.gcs.finalized" = {
      gcs = var.gcs_service_account_email
    }
    "video.validated" = {
      outbox_poller = var.outbox_poller_sa_email
    }
    "video.fanout.thumbnail" = {
      outbox_poller = var.outbox_poller_sa_email
    }
    "video.fanout.transcription" = {
      outbox_poller = var.outbox_poller_sa_email
    }
    "video.fanout.moderation" = {
      outbox_poller = var.outbox_poller_sa_email
    }
    "video.saga.thumbnail" = {
      thumbnail_orch = var.thumbnail_orch_sa_email
    }
    "video.saga.transcription" = {
      transcription_orch = var.transcription_orch_sa_email
    }
    "video.saga.moderation" = {
      outbox_poller = var.outbox_poller_sa_email
    }
    "video.status.update" = {
      outbox_poller = var.outbox_poller_sa_email
    }
  }

  subscription_subscribers = {
    "sub-ingestion" = {
      ingestion = var.ingestion_sa_email
    }
    "sub-transcode-orch" = {
      transcode_orch = var.transcode_orch_sa_email
    }
    "sub-transcode-callback" = {
      transcode_callback = var.transcode_callback_sa_email
    }
    "sub-thumbnail-orch" = {
      thumbnail_orch = var.thumbnail_orch_sa_email
    }
    "sub-transcription-orch" = {
      transcription_orch = var.transcription_orch_sa_email
    }
    "sub-moderation" = {
      moderation_submitter = var.moderation_submitter_sa_email
    }
    "sub-saga-thumbnail" = {
      saga_tracker = var.saga_tracker_sa_email
    }
    "sub-saga-transcription" = {
      saga_tracker = var.saga_tracker_sa_email
    }
    "sub-saga-moderation" = {
      saga_tracker = var.saga_tracker_sa_email
    }
    "sub-notification" = {
      notification = var.notification_sa_email
    }
  }

  topic_publisher_bindings = {
    for binding in flatten([
      for topic_name, members in local.topic_publishers : [
        for service_name, email in members : {
          key    = "${topic_name}-${service_name}"
          topic  = topic_name
          member = email
        }
      ]
    ]) : binding.key => binding
  }

  subscription_subscriber_bindings = {
    for binding in flatten([
      for subscription_name, members in local.subscription_subscribers : [
        for service_name, email in members : {
          key          = "${subscription_name}-${service_name}"
          subscription = subscription_name
          member       = email
        }
      ]
    ]) : binding.key => binding
  }
}

resource "google_pubsub_topic_iam_member" "service_publishers" {
  for_each = local.topic_publisher_bindings

  project = var.project_id
  topic   = google_pubsub_topic.topics[each.value.topic].name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${each.value.member}"
}

resource "google_pubsub_topic_iam_member" "dlq_publisher" {
  project = var.project_id
  topic   = google_pubsub_topic.topics["video.dlq"].name
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_project_service_identity.pubsub.email}"
}

resource "google_pubsub_subscription_iam_member" "service_subscribers" {
  for_each = local.subscription_subscriber_bindings

  project      = var.project_id
  subscription = google_pubsub_subscription.subscriptions[each.value.subscription].name
  role         = "roles/pubsub.subscriber"
  member       = "serviceAccount:${each.value.member}"
}

resource "google_pubsub_subscription_iam_member" "dlq_subscriber" {
  for_each = local.subscriptions

  project      = var.project_id
  subscription = google_pubsub_subscription.subscriptions[each.key].name
  role         = "roles/pubsub.subscriber"
  member       = "serviceAccount:${google_project_service_identity.pubsub.email}"
}
