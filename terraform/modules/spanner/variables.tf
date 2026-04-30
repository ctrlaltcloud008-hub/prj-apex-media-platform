variable "project_id" {
  type        = string
  description = "The unique identifier for the GCP project for resource organization and billing."
  validation {
    condition     = length(var.project_id) > 0
    error_message = "The project_id must not be empty."
  }
}

variable "spanner_instance_name" {
  type        = string
  description = "The unique name for the Spanner instance, used for identification within the project."
  default     = "videopipeline"
}

variable "spanner_instance_config" {
  type        = string
  description = "The configuration for the Spanner instance, specifying the region and replication settings."
}

variable "spanner_instance_display_name" {
  type        = string
  description = "A human-readable name for the Spanner instance, used for display purposes in the GCP console."
  default     = "Video Pipeline"
}

variable "node_count" {
  type        = number
  description = "The number of nodes to allocate for the Spanner instance, determining its performance and capacity."
  default     = 1
}

variable "environment" {
  type        = string
  description = "The environment label for the Spanner instance, used for categorization and filtering in the GCP console."
}

variable "database_name" {
  type        = string
  description = "The name of the Spanner database to be created within the instance, used for organizing and managing data."
  default     = "videopipeline-db"
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

variable "version_retention_period" {
  description = "The version retention period for the Spanner database, specifying how long historical versions of data are retained for recovery and auditing purposes."
  type        = string
  default     = "7d"
}
