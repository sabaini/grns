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

  run "$GRNS_BIN" config get project_prefix
  [ "$status" -eq 0 ]
  [ "$output" = "xx" ]
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

@test "config get ignores project config by default" {
  cd "$BATS_TEST_TMPDIR"
  unset GRNS_CONFIG_DIR
  unset GRNS_TRUST_PROJECT_CONFIG
  cat > .grns.toml <<'EOF'
project_prefix = "xx"
EOF

  run "$GRNS_BIN" config get project_prefix
  [ "$status" -eq 0 ]
  [ "$output" = "gr" ]
}

@test "config get applies project config when trusted" {
  cd "$BATS_TEST_TMPDIR"
  unset GRNS_CONFIG_DIR
  export GRNS_TRUST_PROJECT_CONFIG=true
  cat > .grns.toml <<'EOF'
project_prefix = "xy"
EOF

  run "$GRNS_BIN" config get project_prefix
  [ "$status" -eq 0 ]
  [[ "$output" == *"warning: using trusted project config from"* ]]
  last_line="${output##*$'\n'}"
  [ "$last_line" = "xy" ]
}
