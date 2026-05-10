#!/usr/bin/env bash

set -euo pipefail

services_json="[]"

while IFS= read -r main_file; do
  [[ -n "$main_file" ]] || continue
  service_dir="$(dirname "$(dirname "$main_file")")"
  service_name="$(basename "$service_dir")"
  dockerfile="$service_dir/Dockerfile"
  deployfile="$service_dir/deploy.yaml"

  if [[ ! -f "$dockerfile" ]]; then
    echo "missing Dockerfile for service '$service_name' at $dockerfile" >&2
    exit 1
  fi

  if [[ ! -f "$deployfile" ]]; then
    echo "missing deploy.yaml for service '$service_name' at $deployfile" >&2
    exit 1
  fi

  services_json="$(python3 - "$services_json" "$service_name" "$dockerfile" "$deployfile" <<'PY'
import json
import sys

services = json.loads(sys.argv[1])
services.append({
    "service": sys.argv[2],
    "dockerfile": sys.argv[3],
    "deployfile": sys.argv[4],
    "context": ".",
})
print(json.dumps(services, separators=(",", ":")))
PY
)"
done < <(find services -mindepth 3 -maxdepth 3 -path 'services/*/cmd/main.go' | sort)

printf '%s\n' "$services_json"
