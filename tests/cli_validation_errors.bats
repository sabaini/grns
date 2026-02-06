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
