load 'helpers.bash'
load 'helpers_http.bash'

setup() {
  unset GRNS_DB
  setup_test_env
}

teardown() {
  teardown_test_env
}

@test "api create accepts unknown JSON fields" {
  start_grns_server

  run python3 -c '
import json, os, urllib.request
base = os.environ["GRNS_API_URL"]
payload = {
    "title": "Forward compatibility",
    "priority": 2,
    "future_field": {"nested": True, "version": 1},
    "unknown_scalar": "ok"
}
req = urllib.request.Request(
    base + "/v1/projects/gr/tasks",
    data=json.dumps(payload).encode("utf-8"),
    method="POST",
    headers={"Content-Type": "application/json"},
)
with urllib.request.urlopen(req) as resp:
    assert resp.status == 201
    created = json.loads(resp.read().decode("utf-8"))

task_id = created["id"]
with urllib.request.urlopen(base + f"/v1/projects/gr/tasks/{task_id}") as resp:
    shown = json.loads(resp.read().decode("utf-8"))

assert shown["id"] == task_id
assert shown["title"] == "Forward compatibility"
'
  [ "$status" -eq 0 ]
}

@test "api request body too large returns structured request_too_large" {
  start_grns_server

  run python3 -c '
import json
import os
import urllib.error
import urllib.request

base = os.environ["GRNS_API_URL"]
# /v1/projects/gr/tasks uses defaultJSONMaxBody (1 MiB). Send a payload over that limit.
oversized_title = "x" * (2 * 1024 * 1024)
payload = {"title": oversized_title, "priority": 2}
req = urllib.request.Request(
    base + "/v1/projects/gr/tasks",
    data=json.dumps(payload).encode("utf-8"),
    method="POST",
    headers={"Content-Type": "application/json"},
)

try:
    urllib.request.urlopen(req)
    raise SystemExit("expected HTTP 400")
except urllib.error.HTTPError as e:
    body = json.loads(e.read().decode("utf-8"))
    assert e.code == 400, (e.code, body)
    assert body.get("code") == "invalid_argument", body
    assert body.get("error_code") == 1002, body
'
  [ "$status" -eq 0 ]
}
