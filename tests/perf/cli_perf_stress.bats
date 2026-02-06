load '../helpers.bash'

@test "batch create scales to larger workloads" {
  count="${GRNS_PERF_COUNT:-200}"
  file="$BATS_TEST_TMPDIR/perf_batch.md"

  {
    echo "---"
    echo "type: task"
    echo "priority: 2"
    echo "labels: [perf]"
    echo "---"
    for i in $(seq 1 "$count"); do
      echo "- Perf item $i"
    done
  } > "$file"

  run "$GRNS_BIN" create -f "$file" --json
  [ "$status" -eq 0 ]
  created="$(printf '%s' "$output" | json_array_len)"
  [ "$created" -eq "$count" ]

  run "$GRNS_BIN" list --label perf --json
  [ "$status" -eq 0 ]
  total="$(printf '%s' "$output" | json_array_len)"
  [ "$total" -eq "$count" ]
}
