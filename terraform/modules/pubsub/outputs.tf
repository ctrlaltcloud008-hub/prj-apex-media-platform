output "topic_ids" {
  description = "A map of Pub/Sub topic names to their corresponding IDs."
  value = {
    for name, topic in google_pubsub_topic.topics :
    name => topic.id
  }
}

output "topic_names" {
  description = "A map of Pub/Sub topic names to their corresponding full resource names."
  value = {
    for name, topic in google_pubsub_topic.topics :
    name => topic.name
  }
}

output "gcs_finalized_topic_id" {
  description = "The ID of the Pub/Sub topic for GCS finalized events."
  value       = google_pubsub_topic.topics["video.gcs.finalized"].id
  depends_on  = [google_pubsub_topic_iam_member.service_publishers]
}

output "dlq_topic_id" {
  description = "The ID of the Pub/Sub topic for the dead-letter queue."
  value       = google_pubsub_topic.topics["video.dlq"].id
}

output "transcoder_topic_name" {
  description = "The full resource name of the Pub/Sub topic for video transcoding notifications."
  value       = google_pubsub_topic.topics["video.transcoding.complete"].name
}
