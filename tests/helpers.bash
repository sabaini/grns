setup() {
  setup_test_env
}

teardown() {
  teardown_test_env
}

setup_test_env() {
  export GRNS_BIN="${GRNS_BIN:-$(pwd)/bin/grns}"
  export GRNS_DB="${GRNS_DB:-$BATS_TEST_TMPDIR/grns.db}"
  export GRNS_TEST_DATA_DIR="${GRNS_TEST_DATA_DIR:-$(pwd)/tests/data}"
  if [ -z "${GRNS_API_URL:-}" ]; then
    export GRNS_API_URL="http://127.0.0.1:$(get_free_port)"
  fi
  mkdir -p "$BATS_TEST_TMPDIR"
}

teardown_test_env() {
  if [ -n "${GRNS_TEST_HTTP_PID:-}" ]; then
    kill "$GRNS_TEST_HTTP_PID" 2>/dev/null || true
    wait "$GRNS_TEST_HTTP_PID" 2>/dev/null || true
    unset GRNS_TEST_HTTP_PID
  fi
  unset GRNS_API_URL
  rm -f "$GRNS_DB"
}

json_get() {
  local key="$1"
  python3 -c "import sys, json; data=json.load(sys.stdin); print(data.get('$key',''))"
}

json_has_key() {
  local key="$1"
  python3 -c "import sys, json; data=json.load(sys.stdin); print('true' if '$key' in data else 'false')"
}

json_field_len() {
  local key="$1"
  python3 -c "import sys, json; data=json.load(sys.stdin); value=data.get('$key', None); print(len(value) if isinstance(value, list) else 'missing')"
}

json_array_len() {
  python3 -c "import sys, json; data=json.load(sys.stdin); print(len(data))"
}

json_array_field() {
  local key="$1"
  python3 -c "import sys, json; data=json.load(sys.stdin); print('\n'.join([str(item.get('$key','')) for item in data]))"
}

json_array_field_sorted() {
  local key="$1"
  python3 -c "import sys, json; data=json.load(sys.stdin); values=[str(item.get('$key','')) for item in data]; print('\n'.join(sorted(values)))"
}

json_array_contains_value() {
  local value="$1"
  python3 -c "import sys, json; data=json.load(sys.stdin); print('true' if '$value' in data else 'false')"
}

seed_db() {
  local file="$1"
  python3 - "$file" <<'PY'
import sys
import json
import subprocess
import os

path = sys.argv[1]
bin_path = os.environ.get("GRNS_BIN", "./bin/grns")
env = os.environ.copy()

with open(path, "r", encoding="utf-8") as fh:
    for line in fh:
        line = line.strip()
        if not line:
            continue
        data = json.loads(line)
        args = [bin_path, "create", data["title"], "--json"]
        if data.get("type"):
            args += ["-t", data["type"]]
        if data.get("priority") is not None:
            args += ["-p", str(data["priority"])]
        if data.get("labels"):
            args += ["-l", ",".join(data["labels"])]
        if data.get("spec_id"):
            args += ["--spec-id", data["spec_id"]]
        if data.get("description"):
            args += ["-d", data["description"]]
        subprocess.check_call(args, env=env)
PY
}

wait_for_http_server() {
  local url="$1"
  local timeout_seconds="${2:-5}"
  python3 - "$url" "$timeout_seconds" <<'PY'
import sys
import time
import urllib.request

url = sys.argv[1]
timeout_seconds = float(sys.argv[2])
deadline = time.time() + timeout_seconds
last_error = None

while time.time() < deadline:
    try:
        with urllib.request.urlopen(url, timeout=0.2):
            sys.exit(0)
    except Exception as exc:
        last_error = exc
        time.sleep(0.05)

raise SystemExit(f"server at {url} did not become ready: {last_error}")
PY
}

get_free_port() {
  python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
port = s.getsockname()[1]
s.close()
print(port)
PY
}
