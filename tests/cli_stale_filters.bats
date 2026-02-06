load 'helpers.bash'

@test "stale default excludes closed unless status filter provided" {
  run "$GRNS_BIN" create "Stale open" -t task -p 1 --json
  [ "$status" -eq 0 ]
  open_id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" create "Stale closed" -t task -p 1 --json
  [ "$status" -eq 0 ]
  closed_id="$(printf '%s' "$output" | json_get id)"

  run "$GRNS_BIN" close "$closed_id"  --json
  [ "$status" -eq 0 ]

  python3 - "$GRNS_DB" "$open_id" "$closed_id" <<'PY'
import sqlite3
import sys
import datetime

db, open_id, closed_id = sys.argv[1:4]
old = (datetime.datetime.utcnow() - datetime.timedelta(days=40)).strftime('%Y-%m-%dT%H:%M:%SZ')
conn = sqlite3.connect(db)
conn.execute("UPDATE tasks SET updated_at = ? WHERE id IN (?, ?)", (old, open_id, closed_id))
conn.execute("UPDATE tasks SET closed_at = ? WHERE id = ?", (old, closed_id))
conn.commit()
conn.close()
PY

  run "$GRNS_BIN" stale --json
  [ "$status" -eq 0 ]
  ids="$(printf '%s' "$output" | json_array_field id)"
  echo "$ids" | grep -q "$open_id"
  ! echo "$ids" | grep -q "$closed_id"

  run "$GRNS_BIN" stale --status closed --json
  [ "$status" -eq 0 ]
  ids="$(printf '%s' "$output" | json_array_field id)"
  echo "$ids" | grep -q "$closed_id"
}
