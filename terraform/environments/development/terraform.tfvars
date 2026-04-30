project_id = "apex-494315"
regions = [
  {
    name         = "asia-south1"
    subnet_cidr  = "10.0.0.0/16"
    connect_cidr = "10.123.0.0/28"
  }
]

spanner_instance_config = "regional-asia-south1"
environment             = "development"
edition                 = "ENTERPRISE"

autoscaling = {
  min_nodes         = 1
  max_nodes         = 3
  cpu_target        = 65
  storage_target_db = 70
}
