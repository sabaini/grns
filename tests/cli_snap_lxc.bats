setup() {
  SNAP_LXC_NAME=""
}

teardown() {
  if [ -n "${SNAP_LXC_NAME:-}" ]; then
    lxc delete -f "$SNAP_LXC_NAME" >/dev/null 2>&1 || true
  fi
}

wait_for_lxc_exec() {
  local command="$1"
  local attempts="${2:-60}"
  local sleep_seconds="${3:-1}"

  local i
  for ((i = 0; i < attempts; i++)); do
    if lxc exec "$SNAP_LXC_NAME" -- sh -lc "$command" >/dev/null 2>&1; then
      return 0
    fi
    sleep "$sleep_seconds"
  done

  return 1
}

@test "snap daemon in LXC reports daemon db path via grns info" {
  if [ "${GRNS_RUN_SNAP_LXC_TEST:-0}" != "1" ]; then
    skip "set GRNS_RUN_SNAP_LXC_TEST=1 to enable snap-in-lxc integration test"
  fi

  if ! command -v lxc >/dev/null 2>&1; then
    echo "lxc command not found" >&2
    return 1
  fi

  if ! command -v python3 >/dev/null 2>&1; then
    echo "python3 command not found" >&2
    return 1
  fi

  snap_file="${GRNS_SNAP_FILE:-}"
  if [ -z "$snap_file" ]; then
    snap_file="$(find "$(pwd)" -maxdepth 1 -type f -name 'grns_*.snap' | head -n 1)"
  fi
  if [ -z "$snap_file" ] || [ ! -f "$snap_file" ]; then
    echo "set GRNS_SNAP_FILE to a built grns_*.snap file" >&2
    return 1
  fi

  image="${GRNS_LXC_IMAGE:-ubuntu:24.04}"
  SNAP_LXC_NAME="grns-snap-${BATS_TEST_NUMBER}-$$"

  run lxc launch "$image" "$SNAP_LXC_NAME" -c security.nesting=true
  [ "$status" -eq 0 ]

  run lxc exec "$SNAP_LXC_NAME" -- sh -lc 'if command -v cloud-init >/dev/null 2>&1; then cloud-init status --wait; fi'
  [ "$status" -eq 0 ]

  run lxc exec "$SNAP_LXC_NAME" -- sh -lc 'if ! command -v snap >/dev/null 2>&1; then apt-get update && apt-get install -y snapd; fi'
  [ "$status" -eq 0 ]

  run lxc exec "$SNAP_LXC_NAME" -- sh -lc 'systemctl enable --now snapd.socket >/dev/null 2>&1 || true; systemctl start snapd >/dev/null 2>&1 || true'
  [ "$status" -eq 0 ]

  wait_for_lxc_exec 'snap version >/dev/null 2>&1' 120 1

  run lxc file push "$snap_file" "$SNAP_LXC_NAME/tmp/grns.snap"
  [ "$status" -eq 0 ]

  run lxc exec "$SNAP_LXC_NAME" -- snap install --dangerous /tmp/grns.snap
  [ "$status" -eq 0 ]

  run lxc exec "$SNAP_LXC_NAME" -- snap connect grns:home
  [ "$status" -eq 0 ]
  run lxc exec "$SNAP_LXC_NAME" -- snap connect grns:network
  [ "$status" -eq 0 ]
  run lxc exec "$SNAP_LXC_NAME" -- snap connect grns:network-bind
  [ "$status" -eq 0 ]
  run lxc exec "$SNAP_LXC_NAME" -- snap connect grns:removable-media
  [ "$status" -eq 0 ]

  run lxc exec "$SNAP_LXC_NAME" -- snap start grns.daemon
  [ "$status" -eq 0 ]

  wait_for_lxc_exec '/snap/bin/grns info --json >/tmp/grns-info.json' 120 1

  run lxc exec "$SNAP_LXC_NAME" -- /snap/bin/grns info --json
  [ "$status" -eq 0 ]

  db_path="$(printf '%s' "$output" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("db_path", ""))')"
  [ "$db_path" = "/var/snap/grns/common/grns.db" ]

  run lxc exec "$SNAP_LXC_NAME" -- /snap/bin/grns create "snap lxc smoke" --json
  [ "$status" -eq 0 ]

  run lxc exec "$SNAP_LXC_NAME" -- /snap/bin/grns info --json
  [ "$status" -eq 0 ]

  total_tasks="$(printf '%s' "$output" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("total_tasks", ""))')"
  [ "$total_tasks" = "1" ]
}
