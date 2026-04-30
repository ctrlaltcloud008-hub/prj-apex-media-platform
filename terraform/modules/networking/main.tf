resource "google_compute_network" "vpc" {
  project                 = var.project_id
  name                    = "${var.name}-vpc"
  auto_create_subnetworks = false
  routing_mode            = length(var.regions) > 1 ? "GLOBAL" : "REGIONAL"
}

resource "google_compute_subnetwork" "subnet" {
  for_each                 = { for r in var.regions : r.name => r }
  project                  = var.project_id
  name                     = "${var.name}-subnet-${each.key}"
  network                  = google_compute_network.vpc.id
  region                   = each.key
  ip_cidr_range            = each.value.subnet_cidr
  private_ip_google_access = true
}

resource "google_vpc_access_connector" "connector" {
  for_each      = { for r in var.regions : r.name => r }
  project       = var.project_id
  name          = "apex-con-${each.key}"
  region        = each.key
  network       = google_compute_network.vpc.id
  ip_cidr_range = each.value.connect_cidr
  min_instances = 2
  max_instances = 10
  machine_type  = "e2-standard-4"
}

resource "google_compute_firewall" "allow_internal" {
  project   = var.project_id
  name      = "${var.name}-allow-internal"
  network   = google_compute_network.vpc.id
  direction = "INGRESS"
  priority  = 1000

  allow {
    protocol = "tcp"
    ports    = ["0-65535"]
  }

  allow {
    protocol = "icmp"
  }

  source_ranges = [for r in var.regions : r.subnet_cidr]
}

resource "google_compute_firewall" "allow_health_checks" {
  project   = var.project_id
  name      = "${var.name}-allow-health-checks"
  network   = google_compute_network.vpc.id
  direction = "INGRESS"
  priority  = 900

  allow {
    protocol = "tcp"
    ports    = ["8080", "443"]
  }

  source_ranges = ["35.191.0.0/16", "130.211.0.0/22"]
}

resource "google_compute_firewall" "deny_all_ingress" {
  project   = var.project_id
  name      = "${var.name}-deny-all-ingress"
  network   = google_compute_network.vpc.id
  direction = "INGRESS"
  priority  = 65535

  deny {
    protocol = "all"
  }

  source_ranges = ["0.0.0.0/0"]
}

resource "google_compute_firewall" "allow_google_apis_egress" {
  project   = var.project_id
  name      = "${var.name}-allow-google-apis-egress"
  network   = google_compute_network.vpc.id
  direction = "EGRESS"
  priority  = 1000

  allow {
    protocol = "tcp"
    ports    = ["443"]
  }

  destination_ranges = ["199.36.153.4/30"]
}

resource "google_compute_firewall" "deny_all_egress" {
  project   = var.project_id
  name      = "${var.name}-deny-all-egress"
  network   = google_compute_network.vpc.id
  direction = "EGRESS"
  priority  = 65535

  deny {
    protocol = "all"
  }

  destination_ranges = ["0.0.0.0/0"]
}
