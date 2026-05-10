#!/usr/bin/env python3

import json
import subprocess
import sys
from pathlib import Path

MODE_DEFAULTS = {
    "http": {
        "cpu_idle": True,
        "min_instances": 0,
        "concurrency": 80,
        "timeout_seconds": 300,
        "ingress": "internal",
        "allow_unauthenticated": False,
    },
    "pubsub-pull": {
        "cpu_idle": False,
        "min_instances": 1,
        "concurrency": 10,
        "timeout_seconds": 900,
        "ingress": "internal",
        "allow_unauthenticated": False,
    },
    "poller": {
        "cpu_idle": False,
        "min_instances": 1,
        "concurrency": 10,
        "timeout_seconds": 900,
        "ingress": "internal",
        "allow_unauthenticated": False,
    },
}


def fail(message: str) -> None:
    print(message, file=sys.stderr)
    sys.exit(1)


def main() -> None:
    if len(sys.argv) != 5:
        fail("usage: parse-deploy-config.py <deployfile> <service> <project_id> <environment_name>")

    deployfile = Path(sys.argv[1])
    service = sys.argv[2]
    project_id = sys.argv[3]
    environment_name = sys.argv[4]

    if not deployfile.exists():
        fail(f"missing deploy manifest for service '{service}' at {deployfile}")

    try:
        result = subprocess.run(
            [
                "ruby",
                "-e",
                "require 'yaml'; require 'json'; puts JSON.generate(YAML.safe_load(File.read(ARGV[0]), permitted_classes: [], aliases: false))",
                str(deployfile),
            ],
            check=True,
            capture_output=True,
            text=True,
        )
    except subprocess.CalledProcessError as exc:
        fail(exc.stderr.strip() or exc.stdout.strip() or f"failed to parse YAML manifest {deployfile}")

    data = json.loads(result.stdout)

    if data.get("platform") != "cloud-run":
        fail(f"{deployfile}: platform must be 'cloud-run'")

    execution = data.get("execution") or {}
    mode = execution.get("mode")
    if mode not in MODE_DEFAULTS:
        fail(f"{deployfile}: execution.mode must be one of {', '.join(MODE_DEFAULTS)}")

    defaults = MODE_DEFAULTS[mode]
    scaling = data.get("scaling") or {}
    resources = data.get("resources") or {}
    env = dict(data.get("env") or {})
    secret_env = data.get("secret_env") or {}
    service_name = data.get("service_name", service)
    region = data.get("region", "asia-south1")

    env.setdefault("APP_ENV", environment_name)
    env.setdefault("PROJECT_ID", project_id)
    env.setdefault("SERVICE", service_name)
    env.setdefault("REGION", region)

    parsed = {
        "service_name": service_name,
        "region": region,
        "port": int(data.get("port", 8080)),
        "service_account": data.get("service_account") or f"apex-{service}",
        "ingress": data.get("ingress", defaults["ingress"]),
        "allow_unauthenticated": bool(data.get("allow_unauthenticated", defaults["allow_unauthenticated"])),
        "mode": mode,
        "cpu_idle": bool(execution.get("cpu_idle", defaults["cpu_idle"])),
        "min_instances": int(scaling.get("min_instances", defaults["min_instances"])),
        "max_instances": int(scaling.get("max_instances", max(defaults["min_instances"], 3))),
        "cpu": str(resources.get("cpu", "1")),
        "memory": str(resources.get("memory", "512Mi")),
        "concurrency": int(resources.get("concurrency", defaults["concurrency"])),
        "timeout_seconds": int(resources.get("timeout_seconds", defaults["timeout_seconds"])),
        "env": env,
        "secret_env": secret_env,
        "runtime_service_account_email": f"{(data.get('service_account') or f'apex-{service}')}@{project_id}.iam.gserviceaccount.com",
    }

    print(json.dumps(parsed, separators=(",", ":")))


if __name__ == "__main__":
    main()
