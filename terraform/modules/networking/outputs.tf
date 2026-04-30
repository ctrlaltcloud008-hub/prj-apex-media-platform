output "vpc_id" {
  value = google_compute_network.vpc.id
}

output "subnet_ids" {
  value = { for k, v in google_compute_subnetwork.subnet : k => v.id }
}

output "connector_ids" {
  value = { for k, v in google_vpc_access_connector.connector : k => v.id }
}
