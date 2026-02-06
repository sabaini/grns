load 'helpers.bash'

@test "list supports label and regex spec filters" {
  seed_db "$GRNS_TEST_DATA_DIR/seed.jsonl"

  run "$GRNS_BIN" list --label bug --json
  [ "$status" -eq 0 ]
  titles="$(printf '%s' "$output" | json_array_field title)"
  echo "$titles" | grep -q "Fix auth bug"

  run "$GRNS_BIN" list --label bug,auth --json
  [ "$status" -eq 0 ]
  count="$(printf '%s' "$output" | json_array_len)"
  [ "$count" -eq 1 ]
  titles="$(printf '%s' "$output" | json_array_field title)"
  echo "$titles" | grep -q "Fix auth bug"

  run "$GRNS_BIN" list --spec "auth\\.md" --json
  [ "$status" -eq 0 ]
  titles="$(printf '%s' "$output" | json_array_field title)"
  echo "$titles" | grep -q "Fix auth bug"
}

@test "list supports label-any filters" {
  seed_db "$GRNS_TEST_DATA_DIR/seed.jsonl"

  run "$GRNS_BIN" list --label-any auth,frontend --json
  [ "$status" -eq 0 ]
  titles="$(printf '%s' "$output" | json_array_field title)"
  echo "$titles" | grep -q "Fix auth bug"
  echo "$titles" | grep -q "Add settings page"
}

@test "list supports limit and offset" {
  seed_db "$GRNS_TEST_DATA_DIR/seed.jsonl"

  run "$GRNS_BIN" list --limit 1 --json
  [ "$status" -eq 0 ]
  count="$(printf '%s' "$output" | json_array_len)"
  [ "$count" -eq 1 ]
  title="$(printf '%s' "$output" | json_array_field title)"
  [ "$title" = "Write onboarding docs" ]

  run "$GRNS_BIN" list --limit 1 --offset 1 --json
  [ "$status" -eq 0 ]
  count="$(printf '%s' "$output" | json_array_len)"
  [ "$count" -eq 1 ]
  title="$(printf '%s' "$output" | json_array_field title)"
  [ "$title" = "Add settings page" ]
}

@test "list supports multi-value status filter" {
  run "$GRNS_BIN" create "Open task" -t task -p 1 --json
  [ "$status" -eq 0 ]
  open_id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" create "Closed task" -t task -p 1 --json
  [ "$status" -eq 0 ]
  closed_id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" close "$closed_id"  --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" list --status open,closed --json
  [ "$status" -eq 0 ]
  ids="$(printf '%s' "$output" | json_array_field id)"
  echo "$ids" | grep -q "$open_id"
  echo "$ids" | grep -q "$closed_id"
}
