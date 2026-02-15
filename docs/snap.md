# Snap packaging (Ubuntu)

Grns can be packaged and installed as a snap.

## Build locally

From the repository root:

```bash
snapcraft
```

This produces `grns_*.snap`.

## Install locally

```bash
sudo snap install --dangerous ./grns_*.snap
```

## Commands

- CLI: `grns`
- Daemon service (enabled by default): `grns.daemon`

Check installed services:

```bash
snap services grns
```

## Always-on service (enabled by default)

The daemon starts automatically after install.

If you stopped it, start it again with:

```bash
sudo snap start grns.daemon
sudo snap services grns
```

Stop it when needed:

```bash
sudo snap stop grns.daemon
```

The daemon uses:

- `GRNS_DB=$SNAP_COMMON/grns.db` (typically `/var/snap/grns/common/grns.db`)
- `GRNS_CONFIG_DIR=$SNAP_COMMON`

The bind address is resolved from config/env precedence (`GRNS_API_URL` > `api_url` in `$SNAP_COMMON/.grns.toml` > default `http://127.0.0.1:7333`).

Set `api_url` in `$SNAP_COMMON/.grns.toml` (or `GRNS_API_URL` in daemon env) to control the daemon bind address.

## CI release workflow

GitHub Actions workflow: `.github/workflows/release-snap.yml`

- Push a tag like `v1.2.3` to build and upload a snap artifact (`grns-snap`).
- Use **Run workflow** (manual dispatch) to optionally publish to the Snap Store.

To enable publishing, add this repository secret:

- `SNAPCRAFT_STORE_CREDENTIALS`: exported Snapcraft login token

Manual publish inputs:

- `publish=true`
- `channel=edge|beta|candidate|stable`

## Interfaces and permissions

The snap uses `strict` confinement and declares these interfaces:

- `home` (CLI access to your home workspace)
- `network` (HTTP client/server traffic)
- `network-bind` (bind `grns srv` to local address/port)
- `removable-media` (optional access to `/media` and `/mnt`)

Check interface connections:

```bash
snap connections grns
```

If needed, connect manually:

```bash
sudo snap connect grns:home
sudo snap connect grns:network
sudo snap connect grns:network-bind
sudo snap connect grns:removable-media
```

## Notes

- The CLI app sets `GRNS_CONFIG_DIR=$SNAP_USER_COMMON` so user config is snap-scoped.
- The CLI connects to the server at `GRNS_API_URL`; in snap setups this is typically the running `grns.daemon` service.
- `grns info` reports the database path of the connected server process (for the daemon this is typically `/var/snap/grns/common/grns.db`).

## Optional LXC integration test

A BATS test verifies snap daemon behavior inside an LXC container:

```bash
GRNS_RUN_SNAP_LXC_TEST=1 bats tests/cli_snap_lxc.bats
# or pass an explicit snap file:
GRNS_RUN_SNAP_LXC_TEST=1 GRNS_SNAP_FILE="$(ls -1 grns_*.snap | head -n 1)" bats tests/cli_snap_lxc.bats
# or via just
just test-snap-lxc
```

Notes:
- Requires `lxc` and a working LXD setup on the host.
- Uses image `ubuntu:24.04` by default (override with `GRNS_LXC_IMAGE`).
- Skipped by default in the regular integration suite.
