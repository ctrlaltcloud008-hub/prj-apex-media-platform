#!/usr/bin/env bash

set -euo pipefail

base_ref="${1:-origin/develop}"
head_ref="${2:-HEAD}"
services_json="${3:?services json is required}"

changed_files_json="$(git diff --name-only "$base_ref" "$head_ref" | python3 -c 'import json,sys; print(json.dumps([line.strip() for line in sys.stdin if line.strip()], separators=(",", ":")))')"

python3 - "$services_json" "$changed_files_json" <<'PY'
import json
import subprocess
import sys

services = json.loads(sys.argv[1])
changed_files = json.loads(sys.argv[2])

if not changed_files:
    print('{"include":[]}')
    sys.exit(0)

broad_triggers = (
    'go.mod',
    'go.sum',
    '.github/',
)

all_selected = False
selected = []

for changed in changed_files:
    if changed.startswith(broad_triggers):
        all_selected = True
        break

service_graph = {}
for service in services:
    name = service['service']
    cmd_path = f"./services/{name}/cmd"
    result = subprocess.run(
        ['go', 'list', '-deps', '-f', '{{.ImportPath}}|{{.Dir}}', cmd_path],
        check=True,
        capture_output=True,
        text=True,
    )
    dirs = set()
    for line in result.stdout.splitlines():
        if '|' not in line:
            continue
        _, directory = line.split('|', 1)
        directory = directory.strip()
        if directory:
            dirs.add(directory)
    service_graph[name] = dirs

repo_root = subprocess.run(
    ['git', 'rev-parse', '--show-toplevel'],
    check=True,
    capture_output=True,
    text=True,
).stdout.strip()

for service in services:
    name = service['service']
    if all_selected:
        selected.append(service)
        continue

    dirs = service_graph[name]
    for changed in changed_files:
        changed_abs = f"{repo_root}/{changed}"
        if any(changed_abs == d or changed_abs.startswith(d + '/') for d in dirs):
            selected.append(service)
            break

unique = []
seen = set()
for service in selected:
    name = service['service']
    if name in seen:
        continue
    seen.add(name)
    unique.append(service)

print(json.dumps({"include": unique}, separators=(",", ":")))
PY
