load 'helpers.bash'

@test "export produces JSONL output" {
  # Create a couple tasks first.
  run "$GRNS_BIN" create "Export task one" --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" create "Export task two" -l "label1" --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" export
  [ "$status" -eq 0 ]

  # Should have 2 lines of JSONL.
  line_count=$(echo "$output" | wc -l)
  [ "$line_count" -eq 2 ]

  # Each line should be valid JSON with an id field.
  echo "$output" | head -1 | python3 -c "import sys,json; d=json.load(sys.stdin); assert 'id' in d"
}

@test "export to file" {
  run "$GRNS_BIN" create "File export" --json
  [ "$status" -eq 0 ]

  outfile="$BATS_TEST_TMPDIR/export.jsonl"
  run "$GRNS_BIN" export -o "$outfile"
  [ "$status" -eq 0 ]
  [ -f "$outfile" ]

  # File should contain valid JSONL.
  line_count=$(wc -l < "$outfile")
  [ "$line_count" -ge 1 ]
}

@test "import from JSONL file" {
  # Create a task, export, then import into a fresh DB.
  run "$GRNS_BIN" create "Import me" -l "tag1" --custom env=prod --json
  [ "$status" -eq 0 ]
  id=$(echo "$output" | json_get id)

  outfile="$BATS_TEST_TMPDIR/export.jsonl"
  run "$GRNS_BIN" export -o "$outfile"
  [ "$status" -eq 0 ]

  # Use a new database.
  export GRNS_DB="$BATS_TEST_TMPDIR/import_target.db"

  run "$GRNS_BIN" import -i "$outfile" --json
  [ "$status" -eq 0 ]

  created=$(echo "$output" | json_get created)
  [ "$created" = "1" ]

  # Verify the task exists in the new DB.
  run "$GRNS_BIN" show "$id" --json
  [ "$status" -eq 0 ]

  title=$(echo "$output" | json_get title)
  [ "$title" = "Import me" ]
}

@test "import dry-run does not create tasks" {
  outfile="$BATS_TEST_TMPDIR/dry.jsonl"
  run "$GRNS_BIN" create "Dry run test" --json
  [ "$status" -eq 0 ]
  id=$(echo "$output" | json_get id)

  run "$GRNS_BIN" export -o "$outfile"
  [ "$status" -eq 0 ]

  export GRNS_DB="$BATS_TEST_TMPDIR/dry_target.db"
  run "$GRNS_BIN" import -i "$outfile" --dry-run --json
  [ "$status" -eq 0 ]

  created=$(echo "$output" | json_get created)
  [ "$created" = "1" ]
  dry=$(echo "$output" | json_get dry_run)
  [ "$dry" = "True" ]

  # Task should NOT actually exist.
  run "$GRNS_BIN" show "$id" --json
  [ "$status" -ne 0 ]
}

@test "import dedupe skip skips existing tasks" {
  run "$GRNS_BIN" create "Dedupe test" --json
  [ "$status" -eq 0 ]
  id=$(echo "$output" | json_get id)

  outfile="$BATS_TEST_TMPDIR/dedupe.jsonl"
  run "$GRNS_BIN" export -o "$outfile"
  [ "$status" -eq 0 ]

  # Import into same DB — should skip.
  run "$GRNS_BIN" import -i "$outfile" --dedupe skip --json
  [ "$status" -eq 0 ]

  skipped=$(echo "$output" | json_get skipped)
  [ "$skipped" = "1" ]
  created=$(echo "$output" | json_get created)
  [ "$created" = "0" ]
}

@test "import-export round trip preserves data" {
  # Create task with labels, custom, and deps.
  run "$GRNS_BIN" create "Parent task" --json
  [ "$status" -eq 0 ]
  parent_id=$(echo "$output" | json_get id)

  run "$GRNS_BIN" create "Child task" -l "important" --custom env=staging --deps "$parent_id" --json
  [ "$status" -eq 0 ]
  child_id=$(echo "$output" | json_get id)

  outfile="$BATS_TEST_TMPDIR/roundtrip.jsonl"
  run "$GRNS_BIN" export -o "$outfile"
  [ "$status" -eq 0 ]

  # Import into a fresh DB.
  export GRNS_DB="$BATS_TEST_TMPDIR/roundtrip_target.db"
  run "$GRNS_BIN" import -i "$outfile" --json
  [ "$status" -eq 0 ]
  created=$(echo "$output" | json_get created)
  [ "$created" = "2" ]

  # Verify child task.
  run "$GRNS_BIN" show "$child_id" --json
  [ "$status" -eq 0 ]
  title=$(echo "$output" | json_get title)
  [ "$title" = "Child task" ]
}
