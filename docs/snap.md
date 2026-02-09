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
- Optional daemon service: `grns.daemon`

Check installed services:

```bash
snap services grns
```

## Optional always-on service

Enable the daemon (disabled by default):

```bash
sudo snap start grns.daemon
sudo snap services grns
```

Disable it:

```bash
sudo snap stop grns.daemon
```

The daemon uses:

- `GRNS_API_URL=http://127.0.0.1:7333`
- `GRNS_DB=$SNAP_COMMON/grns.db` (typically `/var/snap/grns/common/grns.db`)
- `GRNS_CONFIG_DIR=$SNAP_COMMON`

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
- If `grns.daemon` is running, the CLI connects to it and does not auto-spawn another server.
