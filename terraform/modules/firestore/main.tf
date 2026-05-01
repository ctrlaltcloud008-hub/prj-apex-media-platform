locals {
  default_labels = merge(var.labels, {
    environment = var.environment
    managed_by  = "terraform"
  })
}

resource "google_project_service" "firebase_rules" {
  project                    = var.project_id
  service                    = "firebaserules.googleapis.com"
  disable_on_destroy         = true
  disable_dependent_services = true
}

resource "google_firestore_database" "main" {
  project     = var.project_id
  name        = "(default)"
  location_id = var.project_region
  type        = "FIRESTORE_NATIVE"

  concurrency_mode        = "OPTIMISTIC"
  delete_protection_state = var.delete_protection_state

  point_in_time_recovery_enablement = "POINT_IN_TIME_RECOVERY_ENABLED"
}

resource "google_firebaserules_ruleset" "firestore" {
  project = var.project_id
  source {
    files {
      name    = "firestore.rules"
      content = <<-EOT
        rules_version = '2';
        service cloud.firestore {
          match /databases/{database}/documents {
            match /videos/{videoId}/status {
              allow read: if request.auth.uid == resource.data.user_id;
              allow write: if false;
            }
          }
        }
      EOT
    }
  }
  depends_on = [google_firestore_database.main]
}

resource "google_firebaserules_release" "release" {
  project      = var.project_id
  name         = "cloud.firestore/database=${google_firestore_database.main.name}"
  ruleset_name = google_firebaserules_ruleset.firestore.name
}

resource "google_firestore_index" "video_status_by_user" {
  project    = var.project_id
  database   = google_firestore_database.main.name
  collection = "status"

  fields {
    field_path = "user_id"
    order      = "ASCENDING"
  }
  fields {
    field_path = "updated_at"
    order      = "DESCENDING"
  }

  depends_on = [google_firestore_database.main]
}
