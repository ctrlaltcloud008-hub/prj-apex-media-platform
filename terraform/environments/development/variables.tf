variable "project_id" {
  type        = string
  description = "The unique identifier for the GCP project for resource organization and billing."
  validation {
    condition     = length(var.project_id) > 0
    error_message = "The project_id must not be empty."
  }
}

variable "project_region" {
  type        = string
  description = "The GCP region where the resources will be deployed, impacting latency and compliance."
  validation {
    condition     = length(var.project_region) > 0
    error_message = "The project_region must be specified."
  }
}

variable "regions" {
  type = list(object({
    name         = string
    subnet_cidr  = string
    connect_cidr = string
  }))
}

variable "spanner_instance_config" {
  type        = string
  description = "The configuration for the Spanner instance, specifying the region and replication settings."
}

variable "environment" {
  type        = string
  description = "The environment label for the Spanner instance, used for categorization and filtering in the GCP console."
}

variable "autoscaling" {
  description = "Autoscalong config. If null, uses static node_count."
  type = object({
    min_nodes         = number
    max_nodes         = number
    cpu_target        = number
    storage_target_db = number
  })
  default = null
}

variable "edition" {
  type        = string
  description = "The edition of the Spanner instance, determining the features and capabilities available. Options include 'standard' and 'enterprise'."
  default     = "STANDARD"
  validation {
    condition     = contains(["STANDARD", "ENTERPRISE", "ENTERPRISE_PLUS"], var.edition)
    error_message = "The edition must be either 'standard' or 'enterprise'."
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

variable "labels" {
  type        = map(string)
  description = "Lables to apply to all resources"
  default     = {}
}

variable "push_endpoints" {
  type        = map(string)
  description = "Map of subscription name to push endpoint URL."
  default     = {}
}

variable "delete_protection_state" {
  type    = string
  default = "Delete protection: DELETE_PROTECTION_ENABLED or DELETE_PROTECTION_DISABLED"
}
