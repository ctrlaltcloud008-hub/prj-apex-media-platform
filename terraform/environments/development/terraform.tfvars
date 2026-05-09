project_id     = "apex-494315"
project_region = "asia-south1"
regions = [
  {
    name         = "asia-south1"
    subnet_cidr  = "10.0.0.0/16"
    connect_cidr = "10.123.0.0/28"
  }
]

spanner_instance_config = "regional-asia-south1"
environment             = "development"
edition                 = "ENTERPRISE"

autoscaling = {
  min_nodes         = 1
  max_nodes         = 3
  cpu_target        = 65
  storage_target_db = 70
}

upload_bucket_regions  = ["asia-south1"]
temp_bucket_regions    = ["asia-south1"]
output_bucket_location = "ASIA"

message_retention_duration = "86400s"
push_endpoints = {
  "sub-transcode-callback" = "https://placeholder.com/transcode-callback"
  "sub-moderation"         = "https://placeholder.com/moderation"
  "sub-saga-thumbnail"     = "https://placeholder.com/saga-thumbnail"
  "sub-saga-transcription" = "https://placeholder.com/saga-transcription"
  "sub-saga-moderation"    = "https://placeholder.com/saga-moderation"
  "sub-notification"       = "https://placeholder.com/notification"
}

delete_protection_state = "DELETE_PROTECTION_DISABLED"
