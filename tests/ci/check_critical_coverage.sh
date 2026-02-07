#!/usr/bin/env bash
set -euo pipefail

profile="$(mktemp)"
trap 'rm -f "$profile"' EXIT

go test ./internal/server ./internal/store -coverprofile="$profile" >/dev/null

python3 - "$profile" <<'PY'
import os
import sys
from collections import defaultdict

profile = sys.argv[1]

thresholds = {
    "internal/server/importer.go": float(os.getenv("GRNS_COVER_MIN_IMPORTER", "70")),
    "internal/server/list_query.go": float(os.getenv("GRNS_COVER_MIN_LIST_QUERY", "55")),
    "internal/server/task_mapper.go": float(os.getenv("GRNS_COVER_MIN_TASK_MAPPER", "70")),
    "internal/server/task_service.go": float(os.getenv("GRNS_COVER_MIN_TASK_SERVICE", "40")),
    "internal/store/tasks.go": float(os.getenv("GRNS_COVER_MIN_STORE_TASKS", "70")),
}

covered = defaultdict(int)
total = defaultdict(int)

with open(profile, "r", encoding="utf-8") as fh:
    header = fh.readline()
    if not header.startswith("mode:"):
        raise SystemExit("invalid coverage profile header")
    for line in fh:
        line = line.strip()
        if not line:
            continue
        loc, stmts, count = line.split()
        file_path = loc.split(":", 1)[0]
        if file_path.startswith("grns/"):
            file_path = file_path[len("grns/"):]
        if file_path.startswith("./"):
            file_path = file_path[2:]
        statements = int(stmts)
        executions = int(count)
        total[file_path] += statements
        if executions > 0:
            covered[file_path] += statements

failures = []
for file_path, minimum in thresholds.items():
    file_total = total[file_path]
    file_covered = covered[file_path]
    pct = 0.0 if file_total == 0 else (100.0 * file_covered / file_total)
    print(f"{file_path}: {pct:.1f}% (min {minimum:.1f}%)")
    if pct + 1e-9 < minimum:
        failures.append((file_path, pct, minimum))

if failures:
    print("\ncritical coverage regression detected:")
    for file_path, pct, minimum in failures:
        print(f"- {file_path}: {pct:.1f}% < {minimum:.1f}%")
    raise SystemExit(1)
PY
