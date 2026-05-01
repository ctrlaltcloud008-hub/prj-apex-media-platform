variable "labels" {
  type        = map(string)
  description = "Lables to apply to all resources"
  default     = {}
}

variable "environment" {
  type        = string
  description = "The environment label for the Spanner instance, used for categorization and filtering in the GCP console."
}

variable "project_id" {
  type        = string
  description = "The unique identifier for the GCP project for resource organization and billing."
  validation {
    condition     = length(var.project_id) > 0
    error_message = "The project_id must not be empty."
  }
}

variable "message_retention_duration" {
  type        = string
  description = "The duration for which messages are retained in Pub/Sub topics, specified in seconds (e.g., '604800s' for 7 days)."
  default     = "604800s" # Default to 7 days
}

variable "dlq_max_delivery_attempts" {
  type        = number
  description = "Max delivery attempts before routing to DLQ"
  default     = 5
}

variable "retry_minimum_backoff" {
  type        = string
  description = "Minimum backoff for retry policy"
  default     = "10s"
}

variable "retry_maximum_backoff" {
  type        = string
  description = "Maximum backoff for retry policy"
  default     = "600s"
}

variable "ack_deadline_seconds" {
  type        = number
  description = "Ack deadline in seconds for pull subscriptions"
  default     = 60
}

variable "push_endpoints" {
  type        = map(string)
  description = "Map of subscription name to push endpoint URL."
  default     = {}
}

variable "gcs_service_account_email" {
  type        = string
  description = "The email of the service account that needs to be granted Pub/Sub publisher permissions for GCS notifications."
}
