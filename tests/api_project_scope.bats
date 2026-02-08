load 'helpers.bash'
load 'helpers_http.bash'

@test "api enforces project scope for labels and dep tree" {
  start_grns_server

  run python3 -c '
import json, os, urllib.error, urllib.request
base = os.environ["GRNS_API_URL"]

# Create task in xy with labels via project-scoped create endpoint.
payload = json.dumps({"id":"xy-scp1","title":"scoped","labels":["secret"]}).encode("utf-8")
req = urllib.request.Request(base + "/v1/projects/xy/tasks", data=payload, method="POST", headers={"Content-Type":"application/json"})
with urllib.request.urlopen(req) as resp:
    assert resp.status == 201, resp.status

# Same-project labels read works.
with urllib.request.urlopen(base + "/v1/projects/xy/tasks/xy-scp1/labels") as resp:
    labels = json.loads(resp.read().decode("utf-8"))
    assert resp.status == 200, resp.status
    assert "secret" in labels, labels

# Cross-project labels read must 404 task_not_found.
try:
    urllib.request.urlopen(base + "/v1/projects/gr/tasks/xy-scp1/labels")
    raise SystemExit("expected HTTP 404 for cross-project labels read")
except urllib.error.HTTPError as e:
    body = e.read().decode("utf-8")
    data = json.loads(body)
    assert e.code == 404, (e.code, body)
    assert data.get("code") == "not_found", data
    assert data.get("error_code") == 2001, data

# Missing/cross-project dep-tree root must 404 task_not_found.
for path in (
    "/v1/projects/gr/tasks/gr-miss/deps/tree",
    "/v1/projects/gr/tasks/xy-scp1/deps/tree",
):
    try:
        urllib.request.urlopen(base + path)
        raise SystemExit(f"expected HTTP 404 for {path}")
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8")
        data = json.loads(body)
        assert e.code == 404, (path, e.code, body)
        assert data.get("code") == "not_found", (path, data)
        assert data.get("error_code") == 2001, (path, data)
'
  [ "$status" -eq 0 ]
}
