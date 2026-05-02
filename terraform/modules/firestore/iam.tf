locals {
  datastore_users = {
    notification = var.notification_sa_email
    sweep_jobs   = var.sweep_jobs_sa_email
  }
}

# Firestore data access is granted via project-level Datastore roles.
resource "google_project_iam_member" "datastore_users" {
  for_each = local.datastore_users

  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${each.value}"
}
