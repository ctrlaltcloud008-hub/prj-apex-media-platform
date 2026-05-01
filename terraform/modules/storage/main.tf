locals {
  default_labels = merge(var.labels, {
    environment = var.environment
    managed_by  = "terraform"
  })

  name_prefix = "${var.project_id}-${var.environment}"
}

resource "google_storage_bucket" "uploads" {
  for_each = toset(var.upload_bucket_regions)

  project  = var.project_id
  name     = "${local.name_prefix}-uploads-${each.value}"
  location = each.value

  storage_class               = "STANDARD"
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"

  versioning {
    enabled = true
  }

  lifecycle_rule {
    condition {
      age = 7
    }

    action {
      type = "AbortIncompleteMultipartUpload"
    }
  }

  lifecycle_rule {
    condition {
      age = 30
    }
    action {
      type = "Delete"
    }
  }

  labels = merge(local.default_labels, {
    purpose = "upload"
    region  = each.value
  })
}

resource "google_storage_notification" "upload_finalize" {
  for_each = google_storage_bucket.uploads

  bucket         = each.value.name
  topic          = var.gcs_notification_topic_id
  event_types    = ["OBJECT_FINALIZE"]
  payload_format = "JSON_API_V1"
}


resource "google_storage_bucket" "transcoded" {
  project  = var.project_id
  name     = "${local.name_prefix}-transcoded"
  location = var.output_bucket_location

  storage_class               = "STANDARD"
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"

  versioning {
    enabled = true
  }

  lifecycle_rule {
    condition {
      age = 30
    }

    action {
      type          = "SetStorageClass"
      storage_class = "NEARLINE"
    }
  }

  lifecycle_rule {
    condition {
      age = 365
    }

    action {
      type          = "SetStorageClass"
      storage_class = "ARCHIVE"
    }
  }

  labels = merge(local.default_labels, {
    purpose = "transcoded"
  })

}

resource "google_storage_bucket" "thumbnails" {
  project  = var.project_id
  name     = "${local.name_prefix}-thumbnails"
  location = var.output_bucket_location

  storage_class               = "STANDARD"
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"

  versioning {
    enabled = true
  }

  labels = merge(local.default_labels, {
    purpose = "thumbnails"
  })
}

resource "google_storage_bucket" "captions" {
  project  = var.project_id
  name     = "${local.name_prefix}-captions"
  location = var.output_bucket_location

  storage_class               = "STANDARD"
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"

  versioning {
    enabled = true
  }

  labels = merge(local.default_labels, {
    purpose = "captions"
  })
}


resource "google_storage_bucket" "temp" {
  for_each = toset(var.temp_bucket_regions)

  project  = var.project_id
  name     = "${local.name_prefix}-temp-${each.value}"
  location = each.value

  storage_class               = "STANDARD"
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"

  versioning {
    enabled = true
  }

  lifecycle_rule {
    condition {
      age = 1
    }
    action {
      type = "Delete"
    }
  }

  labels = merge(local.default_labels, {
    purpose = "temp"
    region  = each.value
  })
}
