locals {
  database_users = {
    upload_api         = var.upload_api_sa_email
    ingestion          = var.ingestion_sa_email
    transcode_orch     = var.transcode_orch_sa_email
    transcode_callback = var.transcode_callback_sa_email
    thumbnail_orch     = var.thumbnail_orch_sa_email
    transcription_orch = var.transcription_orch_sa_email
    moderation_poller  = var.moderation_poller_sa_email
    saga_tracker       = var.saga_tracker_sa_email
    publish_gate       = var.publish_gate_sa_email
    outbox_poller      = var.outbox_poller_sa_email
    sweep_jobs         = var.sweep_jobs_sa_email
  }
}

resource "google_spanner_database_iam_member" "database_users" {
  for_each = local.database_users

  project  = var.project_id
  instance = google_spanner_instance.spanner_instance.name
  database = google_spanner_database.spanner_database.name
  role     = "roles/spanner.databaseUser"
  member   = "serviceAccount:${each.value}"
}
