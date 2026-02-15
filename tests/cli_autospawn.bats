load 'helpers.bash'

setup() {
  export GRNS_TEST_START_SERVER=0
  setup_test_env
}

teardown() {
  teardown_test_env
  unset GRNS_TEST_START_SERVER
}

@test "cli errors when api is not running" {
  port="$(get_free_port)"
  export GRNS_API_URL="http://127.0.0.1:${port}"

  run "$GRNS_BIN" create "Needs server" -t task -p 1 --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "hint: ensure a grns server is running at GRNS_API_URL."
  echo "$output" | grep -q "hint: start local server manually with: grns srv"
}

@test "cli errors when api url points to non-grns service" {
  port="$(get_free_port)"
  python3 -m http.server "$port" >/dev/null 2>&1 &
  GRNS_TEST_HTTP_PID=$!
  export GRNS_TEST_HTTP_PID
  export GRNS_API_URL="http://127.0.0.1:${port}"

  wait_for_http_server "http://127.0.0.1:${port}" 5

  run "$GRNS_BIN" create "Bad api" -t task -p 1 --json
  [ "$status" -ne 0 ]
  echo "$output" | grep -q "api error"
  echo "$output" | grep -q "hint: verify GRNS_API_URL points to a grns server."
}
