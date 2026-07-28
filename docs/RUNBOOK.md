# docs/RUNBOOK.md

## If the microSD card dies

The card is treated as a consumable (see the design spec's Storage
section) — this is the boring, rehearsed path back to a working site.

1. Flash a fresh **Raspberry Pi OS Lite (64-bit), Trixie** image with
   Raspberry Pi Imager, pre-configuring hostname, SSH key, and WiFi via
   the gear icon.
2. SSH in and run `deploy/provision.sh` (copy it over first via
   `scp deploy/provision.sh pi@<new-ip>:~/` or clone the repo).
3. Recreate `/srv/homesite/config.yaml` from `config.example.yaml` —
   WiFi password and Spotify credentials are not in git; pull them from
   your password manager.
4. Recreate `/srv/homesite/backup.env` the same way (restic repository
   password and B2 keys).
5. From the Mac: `make deploy` (builds and `scp`s the binary, and also
   `scp -r`s the whole `deploy/` directory to `/srv/homesite/deploy` —
   this is what puts `backup.sh` at the path
   `homesite-backup.service`'s `ExecStart` expects; installing the
   systemd units themselves isn't automated yet — see step 6).
6. Copy `deploy/homesite.service`, `deploy/homesite-backup.service`, and
   `deploy/homesite-backup.timer` into `/etc/systemd/system/` on the Pi
   (they're already at `/srv/homesite/deploy/` after step 5, so this can
   be done locally on the Pi via `sudo cp /srv/homesite/deploy/*.service
   /srv/homesite/deploy/*.timer /etc/systemd/system/`), then:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable --now homesite
   sudo systemctl enable --now homesite-backup.timer
   ```
7. Restore data:
   ```bash
   restic restore latest --target /srv/homesite/restore-tmp
   cp /srv/homesite/restore-tmp/tmp/homesite-snapshot.db /srv/homesite/data/homesite.db
   cp -r /srv/homesite/restore-tmp/srv/homesite/uploads/* /srv/homesite/uploads/
   sudo chown -R homesite:homesite /srv/homesite/data /srv/homesite/uploads
   sudo systemctl restart homesite
   ```
8. Confirm `curl http://192.168.1.50/healthz` reports `"status":"ok"`
   and a non-zero `disk_used_bytes`, then open the site on a phone.

Target: under an hour from dead card to working site.

## If a backup silently stopped working

`GET /healthz` includes `last_backup`. If it's more than a day old (or
`"never"`), SSH in and run `deploy/backup.sh` manually to see the error —
most likely an expired B2 key, a full `/tmp`, or (if provisioning predates
this fix) a missing `sqlite3` CLI reported as "command not found: sqlite3".
