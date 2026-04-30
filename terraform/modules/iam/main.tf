locals {
  services = {
    upload-api = {
      display_name = "Upload API"
      description  = "Authenticates clients, generates signed URLs, creates video records"
    }
    ingestion = {
      display_name = "Ingestion Service"
      description  = "Validates uploads, extracts metadata, dedup, publishes via outbox"
    }
    transcode-orchestrator = {
      display_name = "Transcode Orchestrator"
      description  = "Rate-limited Transcoder API job submission"
    }
    transcode-callback = {
      display_name = "Transcode Callback"
      description  = "Records transcode completion, fan-out via outbox"
    }
    thumbnail-orchestrator = {
      display_name = "Thumbnail Orchestrator"
      description  = "Rate-limited vertex AI job submission for thumbnail"
    }
    transcription-orchestrator = {
      display_name = "Transcription Orchestrator"
      description  = "Rate-limited vertex AI job submission for transcription"
    }
    saga-tracker = {
      display_name = "Saga Tracker"
      description  = "Agggregates fan-out completion, triggers publish gate"
    }
    notification = {
      display_name = "Notification Service"
      description  = "Writes status updates to Firestore"
    }
    outbox-poller = {
      display_name = "Outbox Poller"
      description  = "Drains transactional outbox to Pub/Sub"
    }
    sweeps = {
      display_name = "Sweeps Jobs"
      description  = "Scheduled recovery jobs for stuck state"
    }
  }
}

resource "google_service_account" "service" {
  for_each = local.services

  project      = var.project_id
  account_id   = "apex-${each.key}"
  display_name = each.value.display_name
  description  = each.value.description
}


locals {
  spanner_users = [
    "upload-api",
    "ingestion",
    "transcode-orchestrator",
    "trancode-callback",
    "thumbnail-orchestrator",
    "transcription-orchestrator",
    "saga-tracker",
    "notification",
    "outbox-poller",
    "sweeps"
  ]
}

resource "google_spanner_database_iam_member" "database_user" {
  for_each = toset(local.spanner_users)

  project  = var.project_id
  instance = var.spanner_instance
  database = var.spanner_database
  role     = "roles/spanner.databaseUser"
  member   = "serviceAccount:${google_service_account.service[each.key].email}"
}

resource "google_storage_bucket_iam_member" "upload_api_creator" {
  for_each = toset(var.upload_bucket_names)

  bucket = each.value
  role   = "roles/storage.objectCreator"
  member = "serviceAccount:${google_service_account.service["upload-api"].email}"
}

resource "google_storage_bucket_iam_member" "ingestion_viewer" {
  for_each = toset(var.upload_bucket_names)

  bucket = each.value
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${google_service_account.service["ingestion"].email}"
}

locals {
  outbox_poller_topics = [
    "video.validated",
    "video.fanout.thumbnail",
    "video.fanout.transcription",
    "video.fanout.moderation",
    "video.saga.moderation",
    "video.status.update"
  ]
}

resource "google_pubsub_topic_iam_member" "outbox_poller_publisher" {
  for_each = toset(local.outbox_poller_topics)

  topic  = var.pubsub_topic_ids[each.key]
  role   = "roles/pubsub.publisher"
  member = "serviceAccount:${google_service_account.service["outbox-poller"].email}"
}

resource "google_project_iam_member" "transcode_orch_transcoder" {
  project = var.project_id
  role    = "roles/transcoder.admin"
  member  = "serviceAccount:${google_service_account.service["transcode-orchestrator"].email}"
}

resource "google_project_iam_member" "thumbnail_orch_vertex" {
  project = var.project_id
  role    = "roles/aiplatform.user"
  member  = "serviceAccount:${google_service_account.service["thumbnail-orchestrator"].email}"
}

resource "google_project_iam_member" "transcription_orch_vertex" {
  project = var.project_id
  role    = "roles/aiplatform.user"
  member  = "serviceAccount:${google_service_account.service["transcription-orchestrator"].email}"
}

resource "google_project_iam_member" "notification_firestore" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.service["notification"].email}"
}

resource "google_storage_bucket_iam_member" "sweeps_viewer_upload" {
  for_each = toset(var.upload_bucket_names)

  bucket = each.value
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${google_service_account.service["sweeps"].email}"
}

resource "google_storage_bucket_iam_member" "sweeps_admin_output" {
  for_each = toset(var.output_bucket_names)

  bucket = each.value
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${google_service_account.service["sweeps"].email}"
}

resource "google_project_iam_member" "sweeps_vertex" {
  project = var.project_id
  role    = "roles/aiplatform.user"
  member  = "serviceAccount:${google_service_account.service["sweeps"].email}"
}

resource "google_project_iam_member" "sweeps_firestore" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.service["sweeps"].email}"
}

resource "google_project_iam_member" "sweeps_pubsub" {
  project = var.project_id
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:${google_service_account.service["sweeps"].email}"
}

resource "google_service_account_iam_member" "workload_identity" {
  for_each = local.services

  service_account_id = google_service_account.service[each.key].name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[default/apex-${each.key}]"
}
