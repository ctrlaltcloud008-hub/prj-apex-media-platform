#!/usr/bin/env python3

import json
import sys
from pathlib import Path

def main() -> None:
    if len(sys.argv) != 3:
        raise SystemExit("usage: render-cloud-run-env.py <config-json> <outfile>")

    config = json.loads(sys.argv[1])
    outfile = Path(sys.argv[2])
    env = config.get("env", {})

    lines = []
    for key, value in env.items():
        encoded = json.dumps(value)
        lines.append(f"{key}: {encoded}")
    outfile.write_text("\n".join(lines) + ("\n" if lines else ""))


if __name__ == "__main__":
    main()
