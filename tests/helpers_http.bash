start_grns_server() {
  local port
  port="$(get_free_port)"
  export GRNS_API_URL="http://127.0.0.1:${port}"

  "$GRNS_BIN" srv >/dev/null 2>&1 &
  GRNS_TEST_HTTP_PID=$!
  export GRNS_TEST_HTTP_PID

  wait_for_http_server "$GRNS_API_URL/health" 5
}

wait_for_file() {
  local path="$1"
  local timeout_seconds="${2:-3}"

  python3 - "$path" "$timeout_seconds" <<'PY'
import os
import sys
import time

path = sys.argv[1]
timeout = float(sys.argv[2])
deadline = time.time() + timeout
while time.time() < deadline:
    if os.path.exists(path):
        sys.exit(0)
    time.sleep(0.05)
raise SystemExit(f"file did not appear in time: {path}")
PY
}

hold_import_limiter_slot() {
  local ready_file="$1"
  local hold_seconds="${2:-3}"

  READY_FILE="$ready_file" HOLD_SECONDS="$hold_seconds" python3 - <<'PY' &
import http.client
import os
import time
import urllib.parse

base = urllib.parse.urlparse(os.environ["GRNS_API_URL"])
ready = os.environ["READY_FILE"]
hold = float(os.environ.get("HOLD_SECONDS", "3"))

conn = http.client.HTTPConnection(base.hostname, base.port, timeout=10)
conn.putrequest("POST", "/v1/import/stream")
conn.putheader("Content-Type", "application/x-ndjson")
conn.putheader("Content-Length", "100000")
conn.endheaders()
# Keep request body incomplete so handler remains occupied.
conn.send(b"{\"id\":\"gr-hold\"")
with open(ready, "w", encoding="utf-8") as fh:
    fh.write("ready")
time.sleep(hold)
conn.close()
PY

  HOLD_IMPORT_PID=$!
  export HOLD_IMPORT_PID
}
