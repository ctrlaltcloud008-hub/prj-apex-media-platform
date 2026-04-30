output "service_account_emails" {
  description = "A map of service account names to their corresponding email addresses."
  value = {
    for name, sa in google_service_account.service :
    name => sa.email
  }
}

output "service_account_names" {
  description = "A map of service account names to their corresponding unique identifiers."
  value = {
    for name, sa in google_service_account.service :
    name => sa.name
  }
}
