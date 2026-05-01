variable "project_id" {
  type        = string
  description = "The unique identifier for the GCP project for resource organization and billing."
  validation {
    condition     = length(var.project_id) > 0
    error_message = "The project_id must not be empty."
  }
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

variable "regions" {
  type = list(object({
    name         = string
    subnet_cidr  = string
    connect_cidr = string
  }))
}

variable "transcoded_bucket_name" {
  type        = string
  description = "The name of the Cloud Storage bucket where transcoded videos will be stored, ensuring proper organization and access control."
}

variable "pubsub_topic_name" {
  type        = string
  description = "The name of the Pub/Sub topic for video processing notifications, enabling asynchronous communication between services."
}
