load 'helpers.bash'
load 'helpers_http.bash'

@test "api returns structured code for invalid id" {
  start_grns_server

  run python3 -c '
import json, os, sys, urllib.error, urllib.request
url = os.environ["GRNS_API_URL"] + "/v1/projects/gr/tasks/invalid"
try:
    urllib.request.urlopen(url)
    raise SystemExit("expected HTTP 400")
except urllib.error.HTTPError as e:
    body = e.read().decode("utf-8")
    data = json.loads(body)
    assert e.code == 400, (e.code, body)
    assert data.get("code") == "invalid_argument", data
    assert data.get("error_code") == 1004, data
'
  [ "$status" -eq 0 ]
}

@test "api returns structured code for not found" {
  start_grns_server

  run python3 -c '
import json, os, urllib.error, urllib.request
url = os.environ["GRNS_API_URL"] + "/v1/projects/gr/tasks/gr-zzzz"
try:
    urllib.request.urlopen(url)
    raise SystemExit("expected HTTP 404")
except urllib.error.HTTPError as e:
    body = e.read().decode("utf-8")
    data = json.loads(body)
    assert e.code == 404, (e.code, body)
    assert data.get("code") == "not_found", data
    assert data.get("error_code") == 2001, data
'
  [ "$status" -eq 0 ]
}

@test "api returns structured code for conflict" {
  start_grns_server

  run "$GRNS_BIN" create "Original task" --id gr-cf01 --json
  [ "$status" -eq 0 ]

  run python3 -c '
import json, os, urllib.error, urllib.request
url = os.environ["GRNS_API_URL"] + "/v1/projects/gr/tasks"
payload = json.dumps({"id":"gr-cf01","title":"Duplicate"}).encode("utf-8")
req = urllib.request.Request(url, data=payload, method="POST", headers={"Content-Type":"application/json"})
try:
    urllib.request.urlopen(req)
    raise SystemExit("expected HTTP 409")
except urllib.error.HTTPError as e:
    body = e.read().decode("utf-8")
    data = json.loads(body)
    assert e.code == 409, (e.code, body)
    assert data.get("code") == "conflict", data
    assert data.get("error_code") == 2101, data
'
  [ "$status" -eq 0 ]
}

@test "api returns structured code for unauthorized and forbidden" {
  export GRNS_API_TOKEN="token"
  export GRNS_ADMIN_TOKEN="admintoken"
  start_grns_server

  run python3 -c '
import json, os, urllib.error, urllib.request
base = os.environ["GRNS_API_URL"]

# Unauthorized: no Authorization header
try:
    urllib.request.urlopen(base + "/v1/projects/gr/tasks")
    raise SystemExit("expected HTTP 401")
except urllib.error.HTTPError as e:
    body = e.read().decode("utf-8")
    data = json.loads(body)
    assert e.code == 401, (e.code, body)
    assert data.get("code") == "unauthorized", data
    assert data.get("error_code") == 3001, data

# Forbidden: has API token but missing admin token
payload = json.dumps({"older_than_days":1,"dry_run":True}).encode("utf-8")
req = urllib.request.Request(base + "/v1/admin/cleanup", data=payload, method="POST", headers={
    "Content-Type": "application/json",
    "Authorization": "Bearer token",
})
try:
    urllib.request.urlopen(req)
    raise SystemExit("expected HTTP 403")
except urllib.error.HTTPError as e:
    body = e.read().decode("utf-8")
    data = json.loads(body)
    assert e.code == 403, (e.code, body)
    assert data.get("code") == "forbidden", data
    assert data.get("error_code") == 3002, data
'
  [ "$status" -eq 0 ]

  unset GRNS_API_TOKEN
  unset GRNS_ADMIN_TOKEN
}

@test "api returns structured code for resource exhausted (429)" {
  start_grns_server

  ready_file="$BATS_TEST_TMPDIR/import-holder.ready"
  rm -f "$ready_file"

  hold_import_limiter_slot "$ready_file" 3
  holder_pid="$HOLD_IMPORT_PID"
  wait_for_file "$ready_file" 3

  run python3 -c '
import json, os, urllib.error, urllib.request
base = os.environ["GRNS_API_URL"]
payload = json.dumps({"tasks":[{"id":"gr-r429","title":"x","status":"open","type":"task","priority":2}]}).encode("utf-8")
req = urllib.request.Request(base + "/v1/projects/gr/import", data=payload, method="POST", headers={"Content-Type":"application/json"})
try:
    urllib.request.urlopen(req)
    raise SystemExit("expected HTTP 429")
except urllib.error.HTTPError as e:
    body = e.read().decode("utf-8")
    data = json.loads(body)
    assert e.code == 429, (e.code, body)
    assert data.get("code") == "resource_exhausted", data
    assert data.get("error_code") == 3003, data
'
  [ "$status" -eq 0 ]

  wait "$holder_pid" || true
}

