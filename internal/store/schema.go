package store

import "database/sql"

const schemaSQL = `
CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  status TEXT NOT NULL,
  type TEXT NOT NULL,
  priority INTEGER NOT NULL,
  description TEXT,
  spec_id TEXT,
  parent_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  closed_at TEXT,
  custom TEXT
);

CREATE TABLE IF NOT EXISTS task_labels (
  task_id TEXT NOT NULL,
  label TEXT NOT NULL,
  UNIQUE(task_id, label),
  FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS task_deps (
  child_id TEXT NOT NULL,
  parent_id TEXT NOT NULL,
  type TEXT NOT NULL,
  UNIQUE(child_id, parent_id, type),
  FOREIGN KEY (child_id) REFERENCES tasks(id) ON DELETE CASCADE,
  FOREIGN KEY (parent_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_tasks_status_updated ON tasks(status, updated_at);
CREATE INDEX IF NOT EXISTS idx_tasks_spec_id ON tasks(spec_id);
CREATE INDEX IF NOT EXISTS idx_tasks_parent_id ON tasks(parent_id);
CREATE INDEX IF NOT EXISTS idx_task_labels_label ON task_labels(label);
CREATE INDEX IF NOT EXISTS idx_task_deps_child ON task_deps(child_id);
CREATE INDEX IF NOT EXISTS idx_task_deps_parent ON task_deps(parent_id);
`

func bootstrapSchema(db *sql.DB) error {
	_, err := db.Exec(schemaSQL)
	return err
}
