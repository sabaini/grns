load 'helpers.bash'

@test "create task with --custom key=value pairs" {
  run "$GRNS_BIN" create "Custom task" --custom env=prod --custom team=backend --json
  [ "$status" -eq 0 ]

  env=$(echo "$output" | python3 -c "import sys,json; print(json.load(sys.stdin).get('custom',{}).get('env',''))")
  [ "$env" = "prod" ]

  team=$(echo "$output" | python3 -c "import sys,json; print(json.load(sys.stdin).get('custom',{}).get('team',''))")
  [ "$team" = "backend" ]
}

@test "create task with --custom-json" {
  run "$GRNS_BIN" create "JSON custom" --custom-json '{"count":42,"active":true}' --json
  [ "$status" -eq 0 ]

  count=$(echo "$output" | python3 -c "import sys,json; print(json.load(sys.stdin).get('custom',{}).get('count',''))")
  [ "$count" = "42" ]
}

@test "show task displays custom fields in JSON" {
  run "$GRNS_BIN" create "Show custom" --custom env=staging --json
  [ "$status" -eq 0 ]
  id=$(echo "$output" | json_get id)

  run "$GRNS_BIN" show "$id" --json
  [ "$status" -eq 0 ]

  env=$(echo "$output" | python3 -c "import sys,json; print(json.load(sys.stdin).get('custom',{}).get('env',''))")
  [ "$env" = "staging" ]
}

@test "update task custom fields" {
  run "$GRNS_BIN" create "Update custom" --json
  [ "$status" -eq 0 ]
  id=$(echo "$output" | json_get id)

  run "$GRNS_BIN" update "$id" --custom region=us-east --json
  [ "$status" -eq 0 ]

  region=$(echo "$output" | python3 -c "import sys,json; print(json.load(sys.stdin).get('custom',{}).get('region',''))")
  [ "$region" = "us-east" ]
}

@test "show task plain output includes custom fields" {
  run "$GRNS_BIN" create "Plain custom" --custom env=dev --json
  [ "$status" -eq 0 ]
  id=$(echo "$output" | json_get id)

  run "$GRNS_BIN" show "$id"
  [ "$status" -eq 0 ]
  echo "$output" | grep -q "env: dev"
}
