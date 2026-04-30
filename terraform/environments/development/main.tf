# module "networking" {
#   source     = "../../modules/networking"
#   project_id = var.project_id
#
#   regions = var.regions
# }
#
# module "spanner" {
#   source     = "../../modules/spanner"
#   project_id = var.project_id
#
#   spanner_instance_config = var.spanner_instance_config
#   environment             = var.environment
#   edition                 = var.edition
#   autoscaling             = var.autoscaling
# }
