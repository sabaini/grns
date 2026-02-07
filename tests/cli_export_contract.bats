load 'helpers.bash'

@test "export rejects --json to keep NDJSON contract explicit" {
  run "$GRNS_BIN" create "Export contract" --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" export --json
  [ "$status" -ne 0 ]
  [[ "$output" == *"export always emits NDJSON; remove --json"* ]]
}
