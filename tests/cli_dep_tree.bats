load 'helpers.bash'

@test "dependency tree shows upstream and downstream" {
  run "$GRNS_BIN" create "Parent task" -t task -p 2 --json
  [ "$status" -eq 0 ]
  parent_id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" create "Child task" -t task -p 2 --json
  [ "$status" -eq 0 ]
  child_id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" dep add "$child_id" "$parent_id" --json
  [ "$status" -eq 0 ]

  # Tree from parent should show downstream.
  run "$GRNS_BIN" dep tree "$parent_id" --json
  [ "$status" -eq 0 ]

  root="$(printf '%s' "$output" | json_get root_id)"
  [ "$root" = "$parent_id" ]

  node_count="$(printf '%s' "$output" | json_field_len nodes)"
  [ "$node_count" = "1" ]

  direction="$(printf '%s' "$output" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['nodes'][0]['direction'])")"
  [ "$direction" = "downstream" ]

  # Tree from child should show upstream.
  run "$GRNS_BIN" dep tree "$child_id" --json
  [ "$status" -eq 0 ]

  direction="$(printf '%s' "$output" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['nodes'][0]['direction'])")"
  [ "$direction" = "upstream" ]
}
