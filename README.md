# README

## Overview

Grns is a lightweight issue tracker and memory system for agents. It’s a pared‑down version of [beads](https://github.com/steveyegge/beads) focused on durable tasks, dependencies, and recency. Use it to track work, model blockers, and surface what’s actionable.

## Architecture

- The CLI talks to a **grns server** via REST.
- In single‑user mode, the CLI **auto‑spawns** a local server backed by embedded SQLite.
- In multi‑user mode, multiple CLIs point at a shared server.

Use `GRNS_API_URL` to point the CLI at an existing server (default `http://127.0.0.1:7333`).

## Core concepts

- **Tasks** have a title, status, priority, labels, and optional spec link.
- **Dependencies** define what *blocks* a task and what it *unblocks*.
- **Ready** tasks are those with **no open blockers**.

## Quickstart

```bash
# Create tasks
grns create "Add auth" -p 0 -t feature -l auth
grns create "Design auth flow" -p 1 -t docs -l auth

# Link dependencies (child depends on parent)
grns dep add <child> <parent>

# Find what’s actionable
grns ready

# Update tasks
grns update <id> --status in_progress
```

## Configuration

Config files (TOML):
- Global: `$HOME/.grns.toml`
- Project: `.grns.toml` in the workspace root (overrides global)

Key settings:
- `project_prefix` (two letters, default `gr`)
- `api_url` (default `http://127.0.0.1:7333`)
- `db_path` (default `.grns.db` in workspace)

Env overrides:
- `GRNS_API_URL`
- `GRNS_DB`

## Examples

These are modeled after beads:

```bash
grns ready                               # List tasks with no open blockers.
grns create "Title" -p 0                # Create a P0 task.
grns dep add <child> <parent>            # Link tasks (blocks, related, parent-child).
grns show <id>                           # View task details and audit trail.

grns stale --days 30 --json                       # Default: 30 days
grns stale --days 90 --status in_progress --json  # Filter by status

grns create "Issue title" -t bug -p 1 -l bug,critical --json
grns create "Issue title" -t bug -p 1 --label bug,critical --json

grns update <id> [<id>...] --status in_progress --json
grns update <id> [<id>...] --priority 1 --json
grns update <id> [<id>...] --spec-id "docs/specs/auth.md" --json
```

## More Examples


### Check Status

```bash
# Check database path and daemon status
grns info --json

# Example output:
# {
#   "database_path": "/path/to/db",
#   "issue_prefix": "grns",
#   "daemon_running": true,
#   "agent_mail_enabled": false
# }
```

### Find Work

```bash
# Find ready work (no blockers)
grns ready --json

# Find stale issues (not updated recently)
grns stale --days 30 --json                    # Default: 30 days
grns stale --days 90 --status in_progress --json  # Filter by status
grns stale --limit 20 --json                   # Limit results
```

## Issue Management

### Create Issues

```bash
# Basic creation
# IMPORTANT: Always quote titles and descriptions with double quotes
grns create "Issue title" -t bug|feature|task -p 0-4 -d "Description" --json

# Create with explicit ID (for parallel workers)
grns create "Issue title" --id worker1-100 -p 1 --json

# Create with labels (--labels or --label work)
grns create "Issue title" -t bug -p 1 -l bug,critical --json
grns create "Issue title" -t bug -p 1 --label bug,critical --json

# Examples with special characters (all require quoting):
grns create "Fix: auth doesn't validate tokens" -t bug -p 1 --json
grns create "Add support for OAuth 2.0" -d "Implement RFC 6749 (OAuth 2.0 spec)" --json
grns create "Implement auth" --spec-id "docs/specs/auth.md" --json

# Create multiple issues from markdown file
grns create -f feature-plan.md --json

# Create with description from file (avoids shell escaping issues)
grns create "Issue title" --body-file=description.md --json
grns create "Issue title" --body-file description.md -p 1 --json

# Read description from stdin
echo "Description text" | grns create "Issue title" --body-file=- --json
cat description.md | grns create "Issue title" --body-file - -p 1 --json

# Create epic with child tasks (IDs remain canonical)
grns create "Auth System" -t epic -p 1 --json                     # Returns: gr-ab12
grns create "Login UI" -p 1 --parent gr-ab12 --json               # Returns: gr-c3d4
grns create "Backend validation" -p 1 --parent gr-ab12 --json     # Returns: gr-d4e5
grns create "Tests" -p 1 --parent gr-ab12 --json                  # Returns: gr-e5f6

# Create and link discovered work (one command)
grns create "Found bug" -t bug -p 1 --deps discovered-from:<parent-id> --json
```

### Update Issues

```bash
# Update one or more issues
grns update <id> [<id>...] --status in_progress --json
grns update <id> [<id>...] --priority 1 --json
grns update <id> [<id>...] --spec-id "docs/specs/auth.md" --json

# Edit issue fields in $EDITOR (HUMANS ONLY - not for agents)
# NOTE: This command is intentionally NOT exposed via the MCP server
# Agents should use 'grns update' with field-specific parameters instead
grns edit <id>                    # Edit description
grns edit <id> --title            # Edit title
grns edit <id> --design           # Edit design notes
grns edit <id> --notes            # Edit notes
grns edit <id> --acceptance       # Edit acceptance criteria
```

### Close/Reopen Issues

```bash
# Complete work (supports multiple IDs)
grns close <id> [<id>...] --reason "Done" --json

# Reopen closed issues (supports multiple IDs)
grns reopen <id> [<id>...] --reason "Reopening" --json
```

### View Issues

```bash
# Show dependency tree
grns dep tree <id>

# Get issue details (supports multiple IDs)
grns show <id> [<id>...] --json
```

## Dependencies & Labels

### Dependencies

```bash
# Link discovered work (old way - two commands)
grns dep add <discovered-id> <parent-id> --type discovered-from

# Create and link in one command (new way - preferred)
grns create "Issue title" -t bug -p 1 --deps discovered-from:<parent-id> --json
```

### Labels

```bash
# Label management (supports multiple IDs)
grns label add <id> [<id>...] <label> --json
grns label remove <id> [<id>...] <label> --json
grns label list <id> --json
grns label list-all --json
```



## Filtering & Search

### Basic Filters

```bash
# Filter by status, priority, type
grns list --status open --priority 1 --json               # Status and priority
grns list --assignee alice --json                         # By assignee
grns list --type bug --json                               # By issue type
grns list --id grns-123,grns-456 --json                       # Specific IDs
grns list --spec "docs/specs/" --json                     # Spec prefix
```

### Label Filters

```bash
# Labels (AND: must have ALL)
grns list --label bug,critical --json

# Labels (OR: has ANY)
grns list --label-any frontend,backend --json
```

### Text Search

```bash
# Title search (substring)
grns list --title "auth" --json

# Pattern matching (case-insensitive substring)
grns list --title-contains "auth" --json                  # Search in title
grns list --desc-contains "implement" --json              # Search in description
grns list --notes-contains "TODO" --json                  # Search in notes
```

### Date Range Filters

```bash
# Date range filters (YYYY-MM-DD or RFC3339)
grns list --created-after 2024-01-01 --json               # Created after date
grns list --created-before 2024-12-31 --json              # Created before date
grns list --updated-after 2024-06-01 --json               # Updated after date
grns list --updated-before 2024-12-31 --json              # Updated before date
grns list --closed-after 2024-01-01 --json                # Closed after date
grns list --closed-before 2024-12-31 --json               # Closed before date
```

### Empty/Null Checks

```bash
# Empty/null checks
grns list --empty-description --json                      # Issues with no description
grns list --no-assignee --json                            # Unassigned issues
grns list --no-labels --json                              # Issues with no labels
```

### Priority Ranges

```bash
# Priority ranges
grns list --priority-min 0 --priority-max 1 --json        # P0 and P1 only
grns list --priority-min 2 --json                         # P2 and below
```

### Combine Filters

```bash
# Combine multiple filters
grns list --status open --priority 1 --label-any urgent,critical --no-assignee --json
```


 
## Advanced Operations

### Cleanup

```bash
# Clean up closed issues (bulk deletion)
grns admin cleanup --force --json                                   # Delete ALL closed issues
grns admin cleanup --older-than 30 --force --json                   # Delete closed >30 days ago
grns admin cleanup --dry-run --json                                 # Preview what would be deleted
grns admin cleanup --older-than 90 --cascade --force --json         # Delete old + dependents
```

### Duplicate Detection & Merging

```bash
# Find and merge duplicate issues
grns duplicates                                          # Show all duplicates
grns duplicates --auto-merge                             # Automatically merge all
grns duplicates --dry-run                                # Preview merge operations

# Merge specific duplicate issues
grns merge <source-id...> --into <target-id> --json      # Consolidate duplicates
grns merge grns-42 grns-43 --into grns-41 --dry-run            # Preview merge
```



## Database Management

### Import/Export

```bash
# Import issues from JSONL
grns import -i issues.jsonl --dry-run      # Preview changes
grns import -i issues.jsonl                # Import and update issues
grns import -i issues.jsonl --dedupe-after # Import + detect duplicates

# Handle missing parents during import
grns import -i issues.jsonl --orphan-handling allow      # Default: import orphans without validation
grns import -i issues.jsonl --orphan-handling resurrect  # Auto-resurrect deleted parents as tombstones
grns import -i issues.jsonl --orphan-handling skip       # Skip orphans with warning
grns import -i issues.jsonl --orphan-handling strict     # Fail if parent is missing

# Configure default orphan handling behavior
grns config set import.orphan_handling "resurrect"
grns sync  # Now uses resurrect mode by default
```


### Migration

```bash
# Migrate databases after version upgrade
grns migrate                                             # Detect and migrate old databases
grns migrate --dry-run                                   # Preview migration
grns migrate --cleanup --yes                             # Migrate and remove old files

# AI-supervised migration (check before running grns migrate)
grns migrate --inspect --json                            # Show migration plan for AI agents
grns info --schema --json                                # Get schema, tables, config, sample IDs
```

### Daemon Management


```bash
# List all running daemons
grns daemons list --json

# Check health (version mismatches, stale sockets)
grns daemons health --json

# Stop/restart specific daemon
grns daemons stop /path/to/workspace --json
grns daemons restart 12345 --json  # By PID

# View daemon logs
grns daemons logs /path/to/workspace -n 100
grns daemons logs 12345 -f  # Follow mode

# Stop all daemons
grns daemons killall --json
grns daemons killall --force --json  # Force kill if graceful fails
```



## Issue Types

- `bug` - Something broken that needs fixing
- `feature` - New functionality
- `task` - Work item (tests, docs, refactoring)
- `epic` - Large feature composed of multiple issues (use `--parent` to link children)
- `chore` - Maintenance work (dependencies, tooling)

## Issue Statuses

- `open` - Ready to be worked on
- `in_progress` - Currently being worked on
- `blocked` - Cannot proceed (waiting on dependencies)
- `deferred` - Deliberately put on ice for later
- `closed` - Work completed
- `tombstone` - Deleted issue (suppresses resurrections)
- `pinned` - Stays open indefinitely (used for hooks, anchors)

**Note:** The `pinned` status is used by orchestrators for hook management and persistent work items that should never be auto-closed or cleaned up.

## Priorities

- `0` - Critical (security, data loss, broken builds)
- `1` - High (major features, important bugs)
- `2` - Medium (nice-to-have features, minor bugs)
- `3` - Low (polish, optimization)
- `4` - Backlog (future ideas)

## Dependency Types

- `blocks` - Hard dependency (issue X blocks issue Y)
- `related` - Soft relationship (issues are connected)
- `parent-child` - Epic/subtask relationship
- `discovered-from` - Track issues discovered during work

Only `blocks` dependencies affect the ready work queue.

**Note:** When creating an issue with a `discovered-from` dependency, the new issue automatically inherits the parent's `source_repo` field.

## Output Formats

JSON is the only machine‑readable output in the MVP (YAML/TOML are post‑MVP).

### JSON Output (Recommended for Agents)

Always use `--json` flag for programmatic use:

```bash
# Single issue
grns show grns-42 --json

# List of issues
grns ready --json

# Operation result
grns create "Issue" -p 1 --json
```

### Human-Readable Output

Default output without `--json`:

```bash
grns ready
# ○ grns-42 [P1] [bug] - Fix authentication bug
# ○ grns-43 [P2] [feature] - Add user settings page
```

**Dependency visibility:** When issues have blocking dependencies, they appear inline:

```bash
grns list --parent gr-ab12
# ○ gr-c3d4 [P1] [task] - Design API (blocks: gr-d4e5, gr-e5f6)
# ○ gr-d4e5 [P1] [task] - Implement endpoints (blocked by: gr-c3d4, blocks: gr-e5f6)
# ○ gr-e5f6 [P1] [task] - Add tests (blocked by: gr-c3d4, gr-d4e5)
```

This makes blocking relationships visible without running `grns show` on each issue.

## Common Patterns for AI Agents

### Claim and Complete Work

```bash
# 1. Find available work
grns ready --json

# 2. Claim issue
grns update grns-42 --status in_progress --json

# 3. Work on it...

# 4. Close when done
grns close grns-42 --reason "Implemented and tested" --json
```

### Discover and Link Work

```bash
# While working on grns-100, discover a bug

# Old way (two commands):
grns create "Found auth bug" -t bug -p 1 --json  # Returns grns-101
grns dep add grns-101 grns-100 --type discovered-from

# New way (one command):
grns create "Found auth bug" -t bug -p 1 --deps discovered-from:grns-100 --json
```

### Batch Operations

```bash
# Update multiple issues at once
grns update grns-41 grns-42 grns-43 --priority 0 --json

# Close multiple issues
grns close grns-41 grns-42 grns-43 --reason "Batch completion" --json

# Add label to multiple issues
grns label add grns-41 grns-42 grns-43 urgent --json
```
