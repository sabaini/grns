load 'helpers.bash'

@test "closed_at omitted for open tasks and present for closed tasks" {
  run "$GRNS_BIN" create "Open issue" -t task -p 1 --json
  [ "$status" -eq 0 ]
  id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" show "$id" --json
  [ "$status" -eq 0 ]
  has_closed_at="$(printf '%s' "$output" | json_has_key closed_at)"
  [ "$has_closed_at" = "false" ]

  run "$GRNS_BIN" close "$id"  --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" show "$id" --json
  [ "$status" -eq 0 ]
  has_closed_at="$(printf '%s' "$output" | json_has_key closed_at)"
  [ "$has_closed_at" = "true" ]
}

@test "labels are always arrays even when empty" {
  run "$GRNS_BIN" create "No labels" -t task -p 1 --json
  [ "$status" -eq 0 ]
  id="$(printf '%s' "$output" | json_get id)"
  has_labels="$(printf '%s' "$output" | json_has_key labels)"
  [ "$has_labels" = "true" ]
  label_len="$(printf '%s' "$output" | json_field_len labels)"
  [ "$label_len" -eq 0 ]

  run "$GRNS_BIN" show "$id" --json
  [ "$status" -eq 0 ]
  has_labels="$(printf '%s' "$output" | json_has_key labels)"
  [ "$has_labels" = "true" ]
  label_len="$(printf '%s' "$output" | json_field_len labels)"
  [ "$label_len" -eq 0 ]
}

@test "deps preserved on create and show" {
  run "$GRNS_BIN" create "Parent" -t task -p 1 --json
  [ "$status" -eq 0 ]
  parent_id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" create "Child" -t task -p 1 --deps "$parent_id" --json
  [ "$status" -eq 0 ]
  child_id="$(printf '%s' "$output" | json_get id)"
  dep_parent="$(printf '%s' "$output" | python3 -c "import sys, json; data=json.load(sys.stdin); print(data['deps'][0]['parent_id'])")"
  dep_type="$(printf '%s' "$output" | python3 -c "import sys, json; data=json.load(sys.stdin); print(data['deps'][0]['type'])")"
  [ "$dep_parent" = "$parent_id" ]
  [ "$dep_type" = "blocks" ]

  run "$GRNS_BIN" show "$child_id" --json
  [ "$status" -eq 0 ]
  dep_parent="$(printf '%s' "$output" | python3 -c "import sys, json; data=json.load(sys.stdin); print(data['deps'][0]['parent_id'])")"
  dep_type="$(printf '%s' "$output" | python3 -c "import sys, json; data=json.load(sys.stdin); print(data['deps'][0]['type'])")"
  [ "$dep_parent" = "$parent_id" ]
  [ "$dep_type" = "blocks" ]
}
