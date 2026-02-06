load 'helpers.bash'

@test "create rejects invalid type" {
  run "$GRNS_BIN" create "Bad type" -t nope --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "invalid type"
}

@test "update rejects invalid status" {
  run "$GRNS_BIN" create "Valid task" -t task -p 1 --json
  [ "$status" -eq 0 ]
  id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" update "$id" --status nope --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "invalid status"
}

@test "priority range enforced on create and update" {
  run "$GRNS_BIN" create "Bad priority" -t task -p 9 --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "priority must be between 0 and 4"

  run "$GRNS_BIN" create "Good priority" -t task -p 1 --json
  [ "$status" -eq 0 ]
  id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" update "$id" --priority 9 --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "priority must be between 0 and 4"
}

@test "list rejects out-of-range priority filters" {
  run "$GRNS_BIN" list --priority 9 --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "priority must be between 0 and 4"
}

@test "invalid ids rejected on show update and dep add" {
  run "$GRNS_BIN" show bad-id --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "invalid id"

  run "$GRNS_BIN" update bad-id --status open --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "invalid id"

  run "$GRNS_BIN" create "Parent" -t task -p 1 --json
  [ "$status" -eq 0 ]
  parent_id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" dep add bad-id "$parent_id" --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "invalid dependency ids"
}

@test "list rejects invalid spec regex" {
  run "$GRNS_BIN" list --spec "[" --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "invalid spec regex"
}

@test "create requires title" {
  run "$GRNS_BIN" create --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "title is required"
}

@test "update requires at least one field" {
  run "$GRNS_BIN" create "No field update" -t task -p 1 --json
  [ "$status" -eq 0 ]
  id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" update "$id" --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "no fields to update"
}

@test "duplicate id returns conflict" {
  run "$GRNS_BIN" create "First" --id gr-ab12 -t task -p 1 --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" create "Second" --id gr-ab12 -t task -p 1 --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "conflict"
}

@test "nonexistent id returns not_found" {
  run "$GRNS_BIN" show gr-zzzz --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "not_found"
}

@test "close and reopen nonexistent id return not_found" {
  run "$GRNS_BIN" close gr-zzzz --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "not_found"

  run "$GRNS_BIN" reopen gr-zzzz --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "not_found"
}

@test "list rejects malformed numeric query params via API" {
  port="$(get_free_port)"
  export GRNS_API_URL="http://127.0.0.1:${port}"

  "$GRNS_BIN" srv >/dev/null 2>&1 &
  GRNS_TEST_HTTP_PID=$!
  export GRNS_TEST_HTTP_PID

  run python3 - <<'PY'
import json
import os
import time
import urllib.error
import urllib.request

base = os.environ["GRNS_API_URL"]
health = base + "/health"
for _ in range(40):
    try:
        with urllib.request.urlopen(health, timeout=0.2):
            break
    except Exception:
        time.sleep(0.05)
else:
    raise SystemExit("server did not start")

url = base + "/v1/tasks?offset=-1"
try:
    urllib.request.urlopen(url)
    raise SystemExit(1)
except urllib.error.HTTPError as e:
    body = e.read().decode('utf-8')
    data = json.loads(body)
    assert e.code == 400
    assert "offset" in data.get("error", "")
PY
  [ "$status" -eq 0 ]
}
