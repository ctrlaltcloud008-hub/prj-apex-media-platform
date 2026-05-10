#!/usr/bin/env bash

set -euo pipefail

ref="$1"

case "$ref" in
  refs/heads/develop)
    echo "environment=development"
    echo "project_secret=GCP_PROJECT_ID_DEV"
    echo "wif_secret=GCP_WORKLOAD_IDENTITY_PROVIDER_DEV"
    echo "sa_secret=GCP_DEPLOY_SERVICE_ACCOUNT_DEV"
    ;;
  refs/heads/main)
    echo "environment=production"
    echo "project_secret=GCP_PROJECT_ID_PROD"
    echo "wif_secret=GCP_WORKLOAD_IDENTITY_PROVIDER_PROD"
    echo "sa_secret=GCP_DEPLOY_SERVICE_ACCOUNT_PROD"
    ;;
  refs/heads/releases/*)
    echo "environment=staging"
    echo "project_secret=GCP_PROJECT_ID_STAGING"
    echo "wif_secret=GCP_WORKLOAD_IDENTITY_PROVIDER_STAGING"
    echo "sa_secret=GCP_DEPLOY_SERVICE_ACCOUNT_STAGING"
    ;;
  *)
    echo "environment="
    echo "project_secret="
    echo "wif_secret="
    echo "sa_secret="
    ;;
esac
