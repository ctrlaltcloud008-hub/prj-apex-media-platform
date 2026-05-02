locals {
  upload_bucket_viewers = {
    ingestion            = var.ingestion_sa_email
    thumbnail_worker     = var.thumbnail_worker_sa_email
    transcription_worker = var.transcription_worker_sa_email
  }

  temp_bucket_creators = {
    thumbnail_worker     = var.thumbnail_worker_sa_email
    transcription_worker = var.transcription_worker_sa_email
  }

  all_bucket_admins = {
    sweep_jobs = var.sweep_jobs_sa_email
  }

  upload_bucket_viewer_bindings = {
    for binding in flatten([
      for bucket_key, bucket in google_storage_bucket.uploads : [
        for service_name, email in local.upload_bucket_viewers : {
          key    = "${bucket_key}-${service_name}"
          bucket = bucket.name
          member = email
        }
      ]
    ]) : binding.key => binding
  }

  temp_bucket_creator_bindings = {
    for binding in flatten([
      for bucket_key, bucket in google_storage_bucket.temp : [
        for service_name, email in local.temp_bucket_creators : {
          key    = "${bucket_key}-${service_name}"
          bucket = bucket.name
          member = email
        }
      ]
    ]) : binding.key => binding
  }

  bucket_names = merge(
    { for bucket_key, bucket in google_storage_bucket.uploads : "uploads-${bucket_key}" => bucket.name },
    {
      transcoded = google_storage_bucket.transcoded.name
      thumbnails = google_storage_bucket.thumbnails.name
      captions   = google_storage_bucket.captions.name
    },
    { for bucket_key, bucket in google_storage_bucket.temp : "temp-${bucket_key}" => bucket.name },
  )

  all_bucket_admin_bindings = {
    for binding in flatten([
      for bucket_key, bucket_name in local.bucket_names : [
        for service_name, email in local.all_bucket_admins : {
          key    = "${bucket_key}-${service_name}"
          bucket = bucket_name
          member = email
        }
      ]
    ]) : binding.key => binding
  }
}

resource "google_storage_bucket_iam_member" "upload_api_creator" {
  for_each = google_storage_bucket.uploads

  bucket = each.value.name
  role   = "roles/storage.objectCreator"
  member = "serviceAccount:${var.upload_api_sa_email}"
}

resource "google_storage_bucket_iam_member" "upload_bucket_viewers" {
  for_each = local.upload_bucket_viewer_bindings

  bucket = each.value.bucket
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${each.value.member}"
}

resource "google_storage_bucket_iam_member" "thumbnail_bucket_creator" {
  bucket = google_storage_bucket.thumbnails.name
  role   = "roles/storage.objectCreator"
  member = "serviceAccount:${var.thumbnail_worker_sa_email}"
}

resource "google_storage_bucket_iam_member" "captions_bucket_creator" {
  bucket = google_storage_bucket.captions.name
  role   = "roles/storage.objectCreator"
  member = "serviceAccount:${var.transcription_worker_sa_email}"
}

resource "google_storage_bucket_iam_member" "temp_bucket_creators" {
  for_each = local.temp_bucket_creator_bindings

  bucket = each.value.bucket
  role   = "roles/storage.objectCreator"
  member = "serviceAccount:${each.value.member}"
}

resource "google_storage_bucket_iam_member" "all_bucket_admins" {
  for_each = local.all_bucket_admin_bindings

  bucket = each.value.bucket
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${each.value.member}"
}
