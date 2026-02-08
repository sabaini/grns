load 'helpers.bash'

@test "git add/list/remove lifecycle via CLI" {
  run "$GRNS_BIN" create "Git ref lifecycle" --source-repo github.com/acme/repo --json
  [ "$status" -eq 0 ]
  id="$(printf '%s' "$output" | json_get id)"
  [ -n "$id" ]

  run "$GRNS_BIN" git add "$id" --relation design_doc --type path --value docs/design.md --json
  [ "$status" -eq 0 ]
  ref_id="$(printf '%s' "$output" | json_get id)"
  [ -n "$ref_id" ]

  run "$GRNS_BIN" git ls "$id" --json
  [ "$status" -eq 0 ]
  [ "$(printf '%s' "$output" | json_array_len)" -eq 1 ]

  run "$GRNS_BIN" git rm "$ref_id" --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" git ls "$id" --json
  [ "$status" -eq 0 ]
  [ "$(printf '%s' "$output" | json_array_len)" -eq 0 ]
}

@test "close with --commit fails before close when repo context is missing" {
  run "$GRNS_BIN" create "Close without repo context" --json
  [ "$status" -eq 0 ]
  id="$(printf '%s' "$output" | json_get id)"
  [ -n "$id" ]

  run "$GRNS_BIN" close "$id" --commit aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "repo is required"

  run "$GRNS_BIN" show "$id" --json
  [ "$status" -eq 0 ]
  [ "$(printf '%s' "$output" | json_get status)" = "open" ]
}
