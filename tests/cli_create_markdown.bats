load 'helpers.bash'

@test "create -f consumes markdown with front matter and list items" {
  run "$GRNS_BIN" create -f "$GRNS_TEST_DATA_DIR/batch.md" --json
  [ "$status" -eq 0 ]

  count="$(printf '%s' "$output" | json_array_len)"
  [ "$count" -eq 2 ]

  titles_sorted="$(printf '%s' "$output" | json_array_field_sorted title)"
  [ "$titles_sorted" = $'Add usage examples\nWrite introduction' ]

  OUTPUT="$output" python3 - <<'PY'
import json
import os

items = json.loads(os.environ["OUTPUT"])
assert len(items) == 2
for item in items:
    assert item["type"] == "task"
    assert item["priority"] == 2
    assert item["spec_id"] == "docs/specs/onboarding.md"
    labels = set(item.get("labels", []))
    assert {"docs", "onboarding"}.issubset(labels)
PY
}
