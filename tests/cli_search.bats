load 'helpers.bash'

@test "search finds tasks by text across fields" {
  run "$GRNS_BIN" create "Authentication module" -t task -p 2 -d "Implement OAuth login" --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" create "Caching layer" -t feature -p 2 -d "Redis integration" --json
  [ "$status" -eq 0 ]

  # Search by title keyword.
  run "$GRNS_BIN" list --search "authentication" --json
  [ "$status" -eq 0 ]
  count="$(printf '%s' "$output" | json_array_len)"
  [ "$count" = "1" ]

  # Search by description keyword.
  run "$GRNS_BIN" list --search "OAuth" --json
  [ "$status" -eq 0 ]
  count="$(printf '%s' "$output" | json_array_len)"
  [ "$count" = "1" ]

  # Search with no results.
  run "$GRNS_BIN" list --search "nonexistent" --json
  [ "$status" -eq 0 ]
  count="$(printf '%s' "$output" | json_array_len)"
  [ "$count" = "0" ]
}

@test "search composes with status filter" {
  run "$GRNS_BIN" create "Searchable open" -t task -p 2 --json
  [ "$status" -eq 0 ]
  id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" close "$id" --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" create "Searchable still open" -t task -p 2 --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" list --search "searchable" --status open --json
  [ "$status" -eq 0 ]
  count="$(printf '%s' "$output" | json_array_len)"
  [ "$count" = "1" ]
}
