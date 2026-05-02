locals {
  transcoder_admins = {
    transcode_orch = var.transcode_orch_sa_email
  }
}

# Transcoder API permissions are exposed at project scope.
resource "google_project_iam_member" "transcoder_admins" {
  for_each = local.transcoder_admins

  project = var.project_id
  role    = "roles/transcoder.admin"
  member  = "serviceAccount:${each.value}"
}
