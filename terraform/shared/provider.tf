terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "7.30.0"
    }
  }
}

provider "google" {
  # Configuration options
  project               = var.project_id
  user_project_override = true
  billing_project       = var.project_id
}
