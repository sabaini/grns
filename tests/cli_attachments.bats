load 'helpers.bash'

setup() {
  unset GRNS_DB
  setup_test_env
}

teardown() {
  teardown_test_env
}

@test "attach managed lifecycle via CLI" {
  run "$GRNS_BIN" create "Attachment lifecycle" --json
  [ "$status" -eq 0 ]
  task_id="$(printf '%s' "$output" | json_get id)"

  infile="$BATS_TEST_TMPDIR/artifact.txt"
  printf 'hello attachment from bats\n' > "$infile"

  run "$GRNS_BIN" attach add "$task_id" "$infile" --kind artifact --label docs --label Docs --json
  [ "$status" -eq 0 ]
  attachment_id="$(printf '%s' "$output" | json_get id)"
  [ -n "$attachment_id" ]

  source_type="$(printf '%s' "$output" | json_get source_type)"
  [ "$source_type" = "managed_blob" ]

  blob_id="$(printf '%s' "$output" | json_get blob_id)"
  [ -n "$blob_id" ]

  OUTPUT="$output" python3 - <<'PY'
import json, os
obj = json.loads(os.environ["OUTPUT"])
assert obj.get("labels") == ["docs"], obj
PY

  run "$GRNS_BIN" attach list "$task_id" --json
  [ "$status" -eq 0 ]
  count="$(printf '%s' "$output" | json_array_len)"
  [ "$count" -eq 1 ]

  run "$GRNS_BIN" attach show "$attachment_id" --json
  [ "$status" -eq 0 ]
  shown_source="$(printf '%s' "$output" | json_get source_type)"
  [ "$shown_source" = "managed_blob" ]

  outfile="$BATS_TEST_TMPDIR/downloaded.txt"
  run "$GRNS_BIN" attach get "$attachment_id" --output "$outfile"
  [ "$status" -eq 0 ]
  [ -f "$outfile" ]
  cmp -s "$infile" "$outfile"

  run "$GRNS_BIN" attach rm "$attachment_id" --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" attach show "$attachment_id" --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "not_found"
}

@test "attach add-link validation via CLI and server" {
  run "$GRNS_BIN" create "Attachment link validation" --json
  [ "$status" -eq 0 ]
  task_id="$(printf '%s' "$output" | json_get id)"

  # CLI-local validation: both --url and --repo-path
  run "$GRNS_BIN" attach add-link "$task_id" --kind artifact --url https://example.com/a --repo-path docs/a.md --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "exactly one of --url or --repo-path is required"

  # CLI-local validation: neither provided
  run "$GRNS_BIN" attach add-link "$task_id" --kind artifact --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "exactly one of --url or --repo-path is required"

  # Server-side validation: invalid URL scheme
  run "$GRNS_BIN" attach add-link "$task_id" --kind artifact --url ftp://example.com/a --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "external_url"

  # Server-side validation: path traversal
  run "$GRNS_BIN" attach add-link "$task_id" --kind artifact --repo-path ../secret.txt --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "repo_path"
}

@test "admin gc-blobs reflects unreferenced blob state" {
  run "$GRNS_BIN" create "Blob GC task" --json
  [ "$status" -eq 0 ]
  task_id="$(printf '%s' "$output" | json_get id)"

  infile="$BATS_TEST_TMPDIR/same-content.txt"
  printf 'same managed content\n' > "$infile"

  run "$GRNS_BIN" attach add "$task_id" "$infile" --kind artifact --json
  [ "$status" -eq 0 ]
  a1_id="$(printf '%s' "$output" | json_get id)"
  b1_id="$(printf '%s' "$output" | json_get blob_id)"

  run "$GRNS_BIN" attach add "$task_id" "$infile" --kind artifact --json
  [ "$status" -eq 0 ]
  a2_id="$(printf '%s' "$output" | json_get id)"
  b2_id="$(printf '%s' "$output" | json_get blob_id)"

  # Same content should dedupe to one blob.
  [ "$b1_id" = "$b2_id" ]

  run "$GRNS_BIN" attach rm "$a1_id" --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" admin gc-blobs --dry-run --json
  [ "$status" -eq 0 ]
  candidates="$(printf '%s' "$output" | json_get candidate_count)"
  [ "$candidates" -eq 0 ]

  run "$GRNS_BIN" attach rm "$a2_id" --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" admin gc-blobs --dry-run --json
  [ "$status" -eq 0 ]
  candidates="$(printf '%s' "$output" | json_get candidate_count)"
  [ "$candidates" -ge 1 ]

  run "$GRNS_BIN" admin gc-blobs --apply --json
  [ "$status" -eq 0 ]
  deleted="$(printf '%s' "$output" | json_get deleted_count)"
  [ "$deleted" -ge 1 ]
}
