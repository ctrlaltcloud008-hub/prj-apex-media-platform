variable "project_id" {
  type        = string
  description = "The unique identifier for the GCP project for resource organization and billing."
  validation {
    condition     = length(var.project_id) > 0
    error_message = "The project_id must not be empty."
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
