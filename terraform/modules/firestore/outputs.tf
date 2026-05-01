output "database_name" {
  value = google_firestore_database.main.name
}

output "database_id" {
  value = google_firestore_database.main.id
}

output "location" {
  value = google_firestore_database.main.location_id
}
