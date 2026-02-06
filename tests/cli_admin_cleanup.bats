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

  # Force cleanup with older-than 1 (recently closed, won't match).
  run "$GRNS_BIN" admin cleanup --older-than 1 --force --json
  [ "$status" -eq 0 ]

  dry="$(printf '%s' "$output" | json_get dry_run)"
  [ "$dry" = "False" ]
}

@test "admin cleanup requires older-than flag" {
  run "$GRNS_BIN" admin cleanup --json
  [ "$status" -ne 0 ]
}
