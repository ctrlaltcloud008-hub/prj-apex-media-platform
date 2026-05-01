# data "google_storage_project_service_account" "gcs_sa" {
#   project = var.project_id
# }
#
# module "iam" {
#   source     = "../../modules/iam"
#   project_id = var.project_id
# }
#
# module "networking" {
#   source     = "../../modules/networking"
#   project_id = var.project_id
#
#   regions = var.regions
# }
#
# module "spanner" {
#   source     = "../../modules/spanner"
#   project_id = var.project_id
#
#   spanner_instance_config = var.spanner_instance_config
#   environment             = var.environment
#   edition                 = var.edition
#   autoscaling             = var.autoscaling
# }
#
# module "pubsub" {
#   source      = "../../modules/pubsub"
#   project_id  = var.project_id
#   environment = var.environment
#
#   labels                     = var.labels
#   message_retention_duration = var.message_retention_duration
#   dlq_max_delivery_attempts  = var.dlq_max_delivery_attempts
#   retry_minimum_backoff      = var.retry_minimum_backoff
#   retry_maximum_backoff      = var.retry_maximum_backoff
#   ack_deadline_seconds       = var.ack_deadline_seconds
#   push_endpoints             = var.push_endpoints
#   gcs_service_account_email  = data.google_storage_project_service_account.gcs_sa.email_address
# }
#
# module "storage" {
#   source      = "../../modules/storage"
#   project_id  = var.project_id
#   environment = var.environment
#
#   upload_bucket_regions     = var.upload_bucket_regions
#   output_bucket_location    = var.output_bucket_location
#   temp_bucket_regions       = var.temp_bucket_regions
#   gcs_notification_topic_id = module.pubsub.gcs_finalized_topic_id
#   labels                    = var.labels
# }
#
# module "firestore" {
#   source         = "../../modules/firestore"
#   project_id     = var.project_id
#   environment    = var.environment
#   project_region = var.project_region
#
#   delete_protection_state = var.delete_protection_state
# }
#
# module "transcode" {
#   source      = "../../modules/transcoder"
#   project_id  = var.project_id
#   environment = var.environment
#   regions     = var.regions
#
#   pubsub_topic_name      = module.pubsub.transcoder_topic_name
#   transcoded_bucket_name = module.storage.transcoder_output_bucket_name
#
# }
