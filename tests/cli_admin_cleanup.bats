load 'helpers.bash'

@test "admin cleanup dry run succeeds and preserves tasks" {
  run "$GRNS_BIN" create "Cleanup test" -t task -p 2 --json
  [ "$status" -eq 0 ]
  id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" close "$id" --json
  [ "$status" -eq 0 ]

  # Dry run with older-than 1 day (recently closed task won't match).
  run "$GRNS_BIN" admin cleanup --older-than 1 --dry-run --json
  [ "$status" -eq 0 ]

  dry="$(printf '%s' "$output" | json_get dry_run)"
  [ "$dry" = "True" ]

  count="$(printf '%s' "$output" | json_get count)"
  [ "$count" = "0" ]

  task_ids_len="$(printf '%s' "$output" | json_field_len task_ids)"
  [ "$task_ids_len" = "0" ]

  # Task should still exist.
  run "$GRNS_BIN" show "$id" --json
  [ "$status" -eq 0 ]
}

@test "admin cleanup without force defaults to dry run" {
  run "$GRNS_BIN" admin cleanup --older-than 1 --json
  [ "$status" -eq 0 ]

  dry="$(printf '%s' "$output" | json_get dry_run)"
  [ "$dry" = "True" ]
}

@test "admin cleanup force runs actual delete" {
  run "$GRNS_BIN" create "Delete me" -t task -p 2 --json
  [ "$status" -eq 0 ]
  id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" close "$id" --json
  [ "$status" -eq 0 ]

  # Re-import task as closed with old updated_at so it matches cleanup cutoff.
  infile="$BATS_TEST_TMPDIR/cleanup_old_closed.jsonl"
  cat > "$infile" <<EOF
{"id":"$id","title":"Delete me","status":"closed","type":"task","priority":2,"created_at":"2020-01-01T00:00:00Z","updated_at":"2020-01-01T00:00:00Z"}
EOF

  run "$GRNS_BIN" import -i "$infile" --dedupe overwrite --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" admin cleanup --older-than 1 --force --json
  [ "$status" -eq 0 ]

  dry="$(printf '%s' "$output" | json_get dry_run)"
  [ "$dry" = "False" ]

  count="$(printf '%s' "$output" | json_get count)"
  [ "$count" = "1" ]

  OUTPUT="$output" python3 - "$id" <<'PY'
import json
import os
import sys

data = json.loads(os.environ["OUTPUT"])
expected = sys.argv[1]
assert expected in data.get("task_ids", [])
PY

  run "$GRNS_BIN" show "$id" --json
  [ "$status" -ne 0 ]
}

@test "admin cleanup requires older-than flag" {
  run "$GRNS_BIN" admin cleanup --json
  [ "$status" -ne 0 ]
}
