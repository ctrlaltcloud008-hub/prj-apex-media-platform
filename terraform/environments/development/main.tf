data "google_storage_project_service_account" "gcs_sa" {
  project = var.project_id
}

module "iam" {
  source     = "../../modules/iam"
  project_id = var.project_id
}

module "networking" {
  source     = "../../modules/networking"
  project_id = var.project_id

  regions = var.regions
}

module "spanner" {
  source     = "../../modules/spanner"
  project_id = var.project_id

  spanner_instance_config     = var.spanner_instance_config
  environment                 = var.environment
  edition                     = var.edition
  autoscaling                 = var.autoscaling
  upload_api_sa_email         = module.iam.service_account_emails["upload-api"]
  ingestion_sa_email          = module.iam.service_account_emails["ingestion"]
  transcode_orch_sa_email     = module.iam.service_account_emails["transcode-orchestrator"]
  transcode_callback_sa_email = module.iam.service_account_emails["transcode-callback"]
  thumbnail_orch_sa_email     = module.iam.service_account_emails["thumbnail-orchestrator"]
  transcription_orch_sa_email = module.iam.service_account_emails["transcription-orch"]
  moderation_poller_sa_email  = module.iam.service_account_emails["moderation-poller"]
  saga_tracker_sa_email       = module.iam.service_account_emails["saga-tracker"]
  publish_gate_sa_email       = module.iam.service_account_emails["publish-gate"]
  outbox_poller_sa_email      = module.iam.service_account_emails["outbox-poller"]
  sweep_jobs_sa_email         = module.iam.service_account_emails["sweep-jobs"]
}

module "pubsub" {
  source      = "../../modules/pubsub"
  project_id  = var.project_id
  environment = var.environment

  labels                        = var.labels
  message_retention_duration    = var.message_retention_duration
  dlq_max_delivery_attempts     = var.dlq_max_delivery_attempts
  retry_minimum_backoff         = var.retry_minimum_backoff
  retry_maximum_backoff         = var.retry_maximum_backoff
  ack_deadline_seconds          = var.ack_deadline_seconds
  push_endpoints                = var.push_endpoints
  gcs_service_account_email     = data.google_storage_project_service_account.gcs_sa.email_address
  ingestion_sa_email            = module.iam.service_account_emails["ingestion"]
  transcode_orch_sa_email       = module.iam.service_account_emails["transcode-orchestrator"]
  transcode_callback_sa_email   = module.iam.service_account_emails["transcode-callback"]
  thumbnail_orch_sa_email       = module.iam.service_account_emails["thumbnail-orchestrator"]
  transcription_orch_sa_email   = module.iam.service_account_emails["transcription-orch"]
  moderation_submitter_sa_email = module.iam.service_account_emails["moderation-submitter"]
  saga_tracker_sa_email         = module.iam.service_account_emails["saga-tracker"]
  notification_sa_email         = module.iam.service_account_emails["notification"]
  outbox_poller_sa_email        = module.iam.service_account_emails["outbox-poller"]
}

module "storage" {
  source      = "../../modules/storage"
  project_id  = var.project_id
  environment = var.environment

  upload_bucket_regions         = var.upload_bucket_regions
  output_bucket_location        = var.output_bucket_location
  temp_bucket_regions           = var.temp_bucket_regions
  gcs_notification_topic_id     = module.pubsub.gcs_finalized_topic_id
  labels                        = var.labels
  upload_api_sa_email           = module.iam.service_account_emails["upload-api"]
  ingestion_sa_email            = module.iam.service_account_emails["ingestion"]
  thumbnail_worker_sa_email     = module.iam.service_account_emails["thumbnail-worker"]
  transcription_worker_sa_email = module.iam.service_account_emails["transcription-worker"]
  sweep_jobs_sa_email           = module.iam.service_account_emails["sweep-jobs"]
}

module "firestore" {
  source         = "../../modules/firestore"
  project_id     = var.project_id
  environment    = var.environment
  project_region = var.project_region

  delete_protection_state = var.delete_protection_state
  notification_sa_email   = module.iam.service_account_emails["notification"]
  sweep_jobs_sa_email     = module.iam.service_account_emails["sweep-jobs"]
}

module "transcode" {
  source      = "../../modules/transcoder"
  project_id  = var.project_id
  environment = var.environment
  regions     = var.regions

  pubsub_topic_name       = module.pubsub.transcoder_topic_name
  transcoded_bucket_name  = module.storage.transcoder_output_bucket_name
  transcode_orch_sa_email = module.iam.service_account_emails["transcode-orchestrator"]
}
