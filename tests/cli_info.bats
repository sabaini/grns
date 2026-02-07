load 'helpers.bash'

@test "info returns schema version and task counts" {
  run "$GRNS_BIN" create "Info test 1" -t task -p 2 --json
  [ "$status" -eq 0 ]
  run "$GRNS_BIN" create "Info test 2" -t bug -p 1 --json
  [ "$status" -eq 0 ]

  run "$GRNS_BIN" info --json
  [ "$status" -eq 0 ]

  version="$(printf '%s' "$output" | json_get schema_version)"
  [ "$version" = "4" ]

  total="$(printf '%s' "$output" | json_get total_tasks)"
  [ "$total" = "2" ]

  open_count="$(printf '%s' "$output" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('task_counts',{}).get('open',0))")"
  [ "$open_count" = "2" ]
}
