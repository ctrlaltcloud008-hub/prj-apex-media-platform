variable "project_id" {
  type        = string
  description = "The unique identifier for the GCP project for resource organization and billing."
  validation {
    condition     = length(var.project_id) > 0
    error_message = "The project_id must not be empty."
  }
}

# variable "spanner_instance" {
#   type        = string
#   description = "The name of the Spanner instance where the database is located."
# }
#
# variable "spanner_database" {
#   type        = string
#   description = "The name of the Spanner database to which IAM members will be added."
# }
#
# variable "upload_bucket_names" {
#   type        = list(string)
#   description = "Regional upload bucket names"
# }
#
# variable "output_bucket_names" {
#   type        = list(string)
#   description = "Output bucket names (transcoded, thumbnails, transcripts)"
# }
#
# variable "pubsub_topic_ids" {
#   type        = map(string)
#   description = "Map of Pub/Sub topic identifiers"
# }
