load 'helpers.bash'

@test "config get returns default project prefix" {
  run "$GRNS_BIN" config get project_prefix
  [ "$status" -eq 0 ]
  [ "$output" = "gr" ]
}

@test "config set and get roundtrip" {
  export GRNS_CONFIG_DIR="$BATS_TEST_TMPDIR"
  cd "$BATS_TEST_TMPDIR"

  run "$GRNS_BIN" config set project_prefix "xx"
  [ "$status" -eq 0 ]

  # Verify file was created.
  [ -f "$BATS_TEST_TMPDIR/.grns.toml" ]
}

@test "config set rejects invalid key" {
  cd "$BATS_TEST_TMPDIR"
  run "$GRNS_BIN" config set invalid_key value
  [ "$status" -ne 0 ]
}

@test "config get rejects invalid key" {
  run "$GRNS_BIN" config get invalid_key
  [ "$status" -ne 0 ]
}
