# docs/RUNBOOK.md

The live Pi is `apollo@apollo` (mDNS hostname `apollo`, currently
`192.168.1.186` on wifi, MAC `dc:a6:32:2b:e6:cf`). See `CLAUDE.md` at the
repo root for the full current-state picture (sudoers scope, domain
resolution, Spotify setup, what's confirmed vs. still pending) — this file
is just the recovery procedure.

## If the microSD card dies

The card is treated as a consumable (see the design spec's Storage
section) — this is the boring, rehearsed path back to a working site.

1. Flash a fresh **Raspberry Pi OS Lite (64-bit), Trixie** image with
   Raspberry Pi Imager, pre-configuring hostname (`apollo`), SSH key, and
   WiFi via the gear icon. Re-copy your SSH public key if the imager's
   own key-injection doesn't match what you use elsewhere
   (`ssh-copy-id -i ~/.ssh/id_ed25519.pub apollo@apollo`).
2. SSH in and run `deploy/provision.sh` (copy it over first via
   `scp deploy/provision.sh apollo@apollo:~/`), run it as root in one
   shot (`sudo bash provision.sh`) so `usermod -aG homesite` picks up
   the right user — **if you instead prefix every line with sudo
   individually, the script's `$(whoami)` resolves to your actual login
   user, which is what you want; running the whole script under one
   `sudo bash` makes `$(whoami)` resolve to `root` instead, adding the
   wrong user to the `homesite` group.** If that happens (apollo can't
   write to `/srv/homesite`), fix it directly:
   `sudo usermod -aG homesite apollo` — group membership only applies to
   new SSH sessions, so just reconnect.
3. Grant the deploy-flow sudoers exception (so `make deploy`'s
   non-interactive `sudo systemctl restart homesite` doesn't hang
   waiting on a password prompt that has nowhere to go):
   ```bash
   cat > /tmp/homesite-sudoers << 'EOF'
   Cmnd_Alias HOMESITE_DEPLOY = /usr/bin/systemctl restart homesite, /usr/bin/systemctl status homesite, /usr/bin/systemctl daemon-reload, /usr/bin/systemctl enable --now homesite, /usr/bin/systemctl enable --now homesite-backup.timer, /usr/bin/systemctl restart homesite-backup.timer, /usr/bin/systemctl status homesite-backup.timer
   apollo ALL=(root) NOPASSWD: HOMESITE_DEPLOY
   EOF
   sudo visudo -c -f /tmp/homesite-sudoers   # validate before installing
   sudo cp /tmp/homesite-sudoers /etc/sudoers.d/homesite-deploy
   sudo chmod 0440 /etc/sudoers.d/homesite-deploy
   ```
4. Recreate `/srv/homesite/config.yaml` from `config.example.yaml` —
   WiFi password and Spotify credentials are not in git; pull them from
   your password manager. Make it group-readable by `homesite` (the
   service runs as that user, not `apollo`):
   `chgrp homesite /srv/homesite/config.yaml && chmod 0640 /srv/homesite/config.yaml`.
5. Recreate `/srv/homesite/backup.env` the same way (restic repository
   password and B2 keys) — only if backups are actually configured; see
   `CLAUDE.md`, they may not be yet.
6. From the Mac: `make deploy` (builds arm64, `scp`s the binary to a
   `.new` path and `mv`s it into place to avoid "text file busy" on the
   running executable, and `scp -r`s the whole `deploy/` directory to
   `/srv/homesite/deploy` — this is what puts `backup.sh` at the path
   `homesite-backup.service`'s `ExecStart` expects; installing the
   systemd units themselves isn't automated — see step 7).
7. Copy `deploy/homesite.service`, `deploy/homesite-mdns.service`,
   `deploy/homesite-backup.service`, and `deploy/homesite-backup.timer`
   into `/etc/systemd/system/` on the Pi (they're already at
   `/srv/homesite/deploy/` after step 6):
   ```bash
   sudo cp /srv/homesite/deploy/*.service /srv/homesite/deploy/*.timer /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable --now homesite
   sudo systemctl enable --now homesite-mdns   # publishes home.local via mDNS
   # sudo systemctl enable --now homesite-backup.timer   # only once backup.env exists
   ```
8. Re-sync `content/` (menus, cats, welcome copy) if it's not already
   current on the card: `scp -r content/* apollo@apollo:/srv/homesite/content/`.
9. Restore uploaded photos/guest book data, if you have a backup:
   ```bash
   restic restore latest --target /srv/homesite/restore-tmp
   cp /srv/homesite/restore-tmp/tmp/homesite-snapshot.db /srv/homesite/data/homesite.db
   cp -r /srv/homesite/restore-tmp/srv/homesite/uploads/* /srv/homesite/uploads/
   sudo chown -R homesite:homesite /srv/homesite/data /srv/homesite/uploads
   sudo systemctl restart homesite
   ```
   (Skip this if there's nothing to restore — as of 2026-07-28 the live
   site's data was intentionally emptied of test content.)
10. Confirm `curl http://apollo.local/healthz` (or `http://home.local/healthz`
    once DNS/mDNS are confirmed working) reports `"status":"ok"` and a
    non-zero `disk_used_bytes`, then open the site on a phone.

Target: under an hour from dead card to working site.

## If a backup silently stopped working

`GET /healthz` includes `last_backup`. If it's more than a day old (or
`"never"`), SSH in and run `deploy/backup.sh` manually to see the error —
most likely an expired B2 key, a full `/tmp`, or (if provisioning predates
this fix) a missing `sqlite3` CLI reported as "command not found: sqlite3".
