output "instance_id" {
  value = google_spanner_instance.spanner_instance.name
}

output "database_id" {
  value = google_spanner_database.spanner_database.name
}

output "database_path" {
  description = "The full path of the Spanner database, in the format: projects/{project_id}/instances/{instance_id}/databases/{database_id}"
  value       = "projects/${var.project_id}/instances/${google_spanner_instance.spanner_instance.name}/databases/${google_spanner_database.spanner_database.name}"
}
