resource "google_spanner_instance" "spanner_instance" {
  project      = var.project_id
  name         = var.spanner_instance_name
  config       = var.spanner_instance_config
  display_name = var.spanner_instance_display_name
  edition      = var.edition


  dynamic "autoscaling_config" {
    for_each = var.autoscaling != null ? [var.autoscaling] : []
    content {
      autoscaling_limits {
        min_nodes = autoscaling_config.value.min_nodes
        max_nodes = autoscaling_config.value.max_nodes
      }
      autoscaling_targets {
        high_priority_cpu_utilization_percent = autoscaling_config.value.cpu_target
        storage_utilization_percent           = autoscaling_config.value.storage_target_db
      }
    }
  }

  num_nodes = var.autoscaling == null ? var.node_count : null

  labels = {
    environment = var.environment
    managed_by  = "terraform"
  }
}

resource "google_spanner_database" "spanner_database" {
  project  = var.project_id
  instance = google_spanner_instance.spanner_instance.name
  name     = var.database_name

  deletion_protection = false

  version_retention_period = var.version_retention_period
}
