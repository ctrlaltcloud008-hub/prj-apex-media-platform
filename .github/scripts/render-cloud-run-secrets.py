#!/usr/bin/env python3

import json
import sys


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit("usage: render-cloud-run-secrets.py <config-json>")

    config = json.loads(sys.argv[1])
    items = []
    for env_name, secret_cfg in (config.get("secret_env") or {}).items():
        if isinstance(secret_cfg, str):
          secret_name = secret_cfg
          version = 'latest'
        else:
          secret_name = secret_cfg["secret"]
          version = secret_cfg.get("version", "latest")
        items.append(f"{env_name}={secret_name}:{version}")

    print(",".join(items))


if __name__ == "__main__":
    main()
