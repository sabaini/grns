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
