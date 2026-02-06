load 'helpers.bash'

@test "ready excludes blocked tasks" {
  run "$GRNS_BIN" create "Parent task" -t task -p 1 --json
  [ "$status" -eq 0 ]
  parent_id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" create "Child task" -t task -p 1 --json
  [ "$status" -eq 0 ]
  child_id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" dep add "$child_id" "$parent_id" --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" ready --json
  [ "$status" -eq 0 ]
  ids="$(printf '%s' "$output" | json_array_field id)"

  echo "$ids" | grep -q "$parent_id"
  ! echo "$ids" | grep -q "$child_id"
}

@test "dep add defaults type to blocks" {
  run "$GRNS_BIN" create "Dep parent" -t task -p 1 --json
  [ "$status" -eq 0 ]
  parent_id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" create "Dep child" -t task -p 1 --json
  [ "$status" -eq 0 ]
  child_id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" dep add "$child_id" "$parent_id" --json
  [ "$status" -eq 0 ]
  dep_type="$(printf '%s' "$output" | python3 -c "import sys, json; data=json.load(sys.stdin); print(data.get('type',''))")"

  [ "$dep_type" = "blocks" ]
}
