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

@test "import with --stream works" {
  run "$GRNS_BIN" create "Stream import me" --json
  [ "$status" -eq 0 ]
  id=$(echo "$output" | json_get id)

  outfile="$BATS_TEST_TMPDIR/export_stream.jsonl"
  run "$GRNS_BIN" export -o "$outfile"
  [ "$status" -eq 0 ]

  export GRNS_DB="$BATS_TEST_TMPDIR/import_stream_target.db"
  run "$GRNS_BIN" import -i "$outfile" --stream --json
  [ "$status" -eq 0 ]
  created=$(echo "$output" | json_get created)
  [ "$created" = "1" ]

  run "$GRNS_BIN" show "$id" --json
  [ "$status" -eq 0 ]
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

@test "import dedupe skip does not rewrite dependencies" {
  run "$GRNS_BIN" create "Parent one" --id gr-pa11 --json
  [ "$status" -eq 0 ]
  run "$GRNS_BIN" create "Parent two" --id gr-pa22 --json
  [ "$status" -eq 0 ]
  run "$GRNS_BIN" create "Child" --id gr-ch11 --deps gr-pa11 --json
  [ "$status" -eq 0 ]

  infile="$BATS_TEST_TMPDIR/import_skip_deps.jsonl"
  cat > "$infile" <<'EOF'
{"id":"gr-ch11","title":"Child","status":"open","type":"task","priority":2,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","deps":[{"parent_id":"gr-pa22","type":"blocks"}]}
EOF

  run "$GRNS_BIN" import -i "$infile" --dedupe skip --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" show gr-ch11 --json
  [ "$status" -eq 0 ]
  dep_parent="$(echo "$output" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['deps'][0]['parent_id'])")"
  [ "$dep_parent" = "gr-pa11" ]
}

@test "import dedupe error does not rewrite dependencies" {
  run "$GRNS_BIN" create "Parent one" --id gr-pa11 --json
  [ "$status" -eq 0 ]
  run "$GRNS_BIN" create "Parent two" --id gr-pa22 --json
  [ "$status" -eq 0 ]
  run "$GRNS_BIN" create "Child" --id gr-ch11 --deps gr-pa11 --json
  [ "$status" -eq 0 ]

  infile="$BATS_TEST_TMPDIR/import_error_deps.jsonl"
  cat > "$infile" <<'EOF'
{"id":"gr-ch11","title":"Child","status":"open","type":"task","priority":2,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","deps":[{"parent_id":"gr-pa22","type":"blocks"}]}
EOF

  run "$GRNS_BIN" import -i "$infile" --dedupe error --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" show gr-ch11 --json
  [ "$status" -eq 0 ]
  dep_parent="$(echo "$output" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['deps'][0]['parent_id'])")"
  [ "$dep_parent" = "gr-pa11" ]
}

@test "import overwrite with explicit empty deps clears dependencies" {
  run "$GRNS_BIN" create "Parent" --id gr-pa11 --json
  [ "$status" -eq 0 ]
  run "$GRNS_BIN" create "Child" --id gr-ch11 --deps gr-pa11 --json
  [ "$status" -eq 0 ]

  infile="$BATS_TEST_TMPDIR/import_clear_deps.jsonl"
  cat > "$infile" <<'EOF'
{"id":"gr-ch11","title":"Child","status":"open","type":"task","priority":2,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","deps":[]}
EOF

  run "$GRNS_BIN" import -i "$infile" --dedupe overwrite --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" show gr-ch11 --json
  [ "$status" -eq 0 ]
  dep_count="$(echo "$output" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('deps', [])))")"
  [ "$dep_count" -eq 0 ]
}

@test "import overwrite without deps field preserves dependencies" {
  run "$GRNS_BIN" create "Parent" --id gr-pa11 --json
  [ "$status" -eq 0 ]
  run "$GRNS_BIN" create "Child" --id gr-ch11 --deps gr-pa11 --json
  [ "$status" -eq 0 ]

  infile="$BATS_TEST_TMPDIR/import_preserve_deps.jsonl"
  cat > "$infile" <<'EOF'
{"id":"gr-ch11","title":"Child renamed","status":"open","type":"task","priority":2,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}
EOF

  run "$GRNS_BIN" import -i "$infile" --dedupe overwrite --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" show gr-ch11 --json
  [ "$status" -eq 0 ]
  dep_parent="$(echo "$output" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['deps'][0]['parent_id'])")"
  [ "$dep_parent" = "gr-pa11" ]
}

@test "import validates status and rejects invalid values" {
  infile="$BATS_TEST_TMPDIR/import_invalid_status.jsonl"
  cat > "$infile" <<'EOF'
{"id":"gr-aa11","title":"Bad status","status":"nope","type":"task","priority":2,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}
EOF

  run "$GRNS_BIN" import -i "$infile" --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "invalid status"
}

@test "import overwrite normalizes closed status with closed_at" {
  run "$GRNS_BIN" create "Task" --id gr-aa11 --json
  [ "$status" -eq 0 ]

  infile="$BATS_TEST_TMPDIR/import_closed_status.jsonl"
  cat > "$infile" <<'EOF'
{"id":"gr-aa11","title":"Task","status":"closed","type":"task","priority":2,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}
EOF

  run "$GRNS_BIN" import -i "$infile" --dedupe overwrite --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" show gr-aa11 --json
  [ "$status" -eq 0 ]
  status_value="$(echo "$output" | json_get status)"
  [ "$status_value" = "closed" ]
  has_closed_at="$(echo "$output" | json_has_key closed_at)"
  [ "$has_closed_at" = "true" ]
}

@test "import overwrite clears closed_at when reopening" {
  run "$GRNS_BIN" create "Task" --id gr-aa11 --json
  [ "$status" -eq 0 ]
  run "$GRNS_BIN" close gr-aa11 --json
  [ "$status" -eq 0 ]

  infile="$BATS_TEST_TMPDIR/import_open_status.jsonl"
  cat > "$infile" <<'EOF'
{"id":"gr-aa11","title":"Task","status":"open","type":"task","priority":2,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}
EOF

  run "$GRNS_BIN" import -i "$infile" --dedupe overwrite --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" show gr-aa11 --json
  [ "$status" -eq 0 ]
  status_value="$(echo "$output" | json_get status)"
  [ "$status_value" = "open" ]
  has_closed_at="$(echo "$output" | json_has_key closed_at)"
  [ "$has_closed_at" = "false" ]
}

@test "import reports invalid JSON line with line number" {
  infile="$BATS_TEST_TMPDIR/import_invalid_line.jsonl"
  cat > "$infile" <<'EOF'
{"id":"gr-aa11","title":"Good","status":"open","type":"task","priority":2,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}
{"id":"gr-bb22","title":"Bad",
EOF

  run "$GRNS_BIN" import -i "$infile" --stream --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "line 2"
}
