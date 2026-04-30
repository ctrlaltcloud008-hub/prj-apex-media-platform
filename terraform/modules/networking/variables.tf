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

variable "name" {
  type        = string
  default     = "apex-pipeline"
  description = "The name of the Virtual Private Cloud (VPC) network to be created for resource isolation."
}
