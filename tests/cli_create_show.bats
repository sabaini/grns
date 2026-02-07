load 'helpers.bash'

@test "create and show issue" {
  run "$GRNS_BIN" create "Test issue" -t task -p 1 -d "First issue" --json
  [ "$status" -eq 0 ]

  id="$(printf '%s' "$output" | json_get id)"
  [ -n "$id" ]

  run "$GRNS_BIN" show "$id" --json
  [ "$status" -eq 0 ]

  title="$(printf '%s' "$output" | json_get title)"
  [ "$title" = "Test issue" ]
}

@test "update, close, and reopen issue" {
  run "$GRNS_BIN" create "Lifecycle issue" -t task -p 2 --json
  [ "$status" -eq 0 ]
  id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" update "$id" --status in_progress --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" show "$id" --json
  [ "$status" -eq 0 ]
  status_value="$(printf '%s' "$output" | json_get status)"
  [ "$status_value" = "in_progress" ]

  run "$GRNS_BIN" close "$id"  --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" show "$id" --json
  [ "$status" -eq 0 ]
  status_value="$(printf '%s' "$output" | json_get status)"
  [ "$status_value" = "closed" ]

  run "$GRNS_BIN" reopen "$id"  --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" show "$id" --json
  [ "$status" -eq 0 ]
  status_value="$(printf '%s' "$output" | json_get status)"
  [ "$status_value" = "open" ]
}

@test "show multiple ids uses bulk endpoint semantics" {
  run "$GRNS_BIN" create "Show bulk one" --id gr-sh11 --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" create "Show bulk two" --id gr-sh22 --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" show gr-sh22 gr-sh11 --json
  [ "$status" -eq 0 ]

  OUTPUT="$output" python3 - <<'PY'
import json
import os

items = json.loads(os.environ["OUTPUT"])
assert len(items) == 2
assert items[0]["id"] == "gr-sh22"
assert items[1]["id"] == "gr-sh11"
PY

  run "$GRNS_BIN" show gr-sh11 gr-sh99 --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "not_found"
}
