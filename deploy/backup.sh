#!/usr/bin/env bash
# deploy/backup.sh — nightly, via homesite-backup.timer.
set -euo pipefail

DATA_DIR=/srv/homesite/data
UPLOADS_DIR=/srv/homesite/uploads
SNAPSHOT=/tmp/homesite-snapshot.db

rm -f "$SNAPSHOT"
sqlite3 "$DATA_DIR/homesite.db" "VACUUM INTO '$SNAPSHOT';"

restic backup "$SNAPSHOT" "$UPLOADS_DIR" --tag homesite

rm -f "$SNAPSHOT"

date -u +%Y-%m-%dT%H:%M:%SZ | sudo -u homesite tee "$DATA_DIR/.last_backup" > /dev/null
