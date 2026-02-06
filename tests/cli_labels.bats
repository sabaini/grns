load 'helpers.bash'

@test "label add/remove/list" {
  run "$GRNS_BIN" create "Label issue" -t task -p 1 --json
  [ "$status" -eq 0 ]
  id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" label add "$id" urgent --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" label list "$id" --json
  [ "$status" -eq 0 ]
  has_label="$(printf '%s' "$output" | json_array_contains_value urgent)"
  [ "$has_label" = "true" ]

  run "$GRNS_BIN" label remove "$id" urgent --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" label list "$id" --json
  [ "$status" -eq 0 ]
  has_label="$(printf '%s' "$output" | json_array_contains_value urgent)"
  [ "$has_label" = "false" ]
}

@test "label add normalizes case and is idempotent" {
  run "$GRNS_BIN" create "Case label" -t task -p 2 --json
  [ "$status" -eq 0 ]
  id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" label add "$id" Urgent --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" label add "$id" urgent --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" label list "$id" --json
  [ "$status" -eq 0 ]
  count="$(printf '%s' "$output" | json_array_len)"
  [ "$count" -eq 1 ]
  has_label="$(printf '%s' "$output" | json_array_contains_value urgent)"
  [ "$has_label" = "true" ]
}
