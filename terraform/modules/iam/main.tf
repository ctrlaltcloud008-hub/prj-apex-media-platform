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
    thumbnail-worker = {
      display_name = "Thumbnail Worker"
      description  = "Reads uploads and writes generated thumbnails to storage"
    }
    transcription-orch = {
      display_name = "Transcription Orchestrator"
      description  = "Rate-limited vertex AI job submission for transcription"
    }
    transcription-worker = {
      display_name = "Transcription Worker"
      description  = "Reads uploads and writes generated captions to storage"
    }
    moderation-submitter = {
      display_name = "Moderation Submitter"
      description  = "Consumes moderation work items and submits jobs for analysis"
    }
    moderation-poller = {
      display_name = "Moderation Poller"
      description  = "Polls moderation job results and updates pipeline state"
    }
    saga-tracker = {
      display_name = "Saga Tracker"
      description  = "Agggregates fan-out completion, triggers publish gate"
    }
    publish-gate = {
      display_name = "Publish Gate"
      description  = "Verifies publish readiness and finalizes video availability"
    }
    notification = {
      display_name = "Notification Service"
      description  = "Writes status updates to Firestore"
    }
    outbox-poller = {
      display_name = "Outbox Poller"
      description  = "Drains transactional outbox to Pub/Sub"
    }
    sweep-jobs = {
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
