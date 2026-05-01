variable "project_id" {
  type        = string
  description = "The unique identifier for the GCP project for resource organization and billing."
  validation {
    condition     = length(var.project_id) > 0
    error_message = "The project_id must not be empty."
  }
}

variable "upload_bucket_regions" {
  type        = list(string)
  description = "List of regions for upload buckets (one bucket per region)"
}

variable "output_bucket_location" {
  description = "Multi-region locaition for output buckets."
  type        = string
}

variable "temp_bucket_regions" {
  type        = list(string)
  description = "List of regions for temp buckets (one bucket per region)"
}

variable "gcs_notification_topic_id" {
  type        = string
  description = "The ID of the Pub/Sub topic to which GCS notifications will be sent when objects are finalized in the upload buckets."
}

variable "labels" {
  type        = map(string)
  description = "Lables to apply to all resources"
  default     = {}
}

variable "environment" {
  type        = string
  description = "The environment label for the Spanner instance, used for categorization and filtering in the GCP console."
}
