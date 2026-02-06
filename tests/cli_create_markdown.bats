load 'helpers.bash'

@test "create -f consumes markdown with front matter and list items" {
  run "$GRNS_BIN" create -f "$GRNS_TEST_DATA_DIR/batch.md" --json
  [ "$status" -eq 0 ]

  count="$(printf '%s' "$output" | json_array_len)"
  [ "$count" -eq 2 ]

  titles="$(printf '%s' "$output" | json_array_field title)"
  echo "$titles" | grep -q "Write introduction"
  echo "$titles" | grep -q "Add usage examples"
}
