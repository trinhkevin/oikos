#!/usr/bin/env bash
# deploy/provision.sh — run once on a fresh Raspberry Pi OS Lite (Trixie) install.
set -euo pipefail

sudo apt update
sudo apt install -y imagemagick restic sqlite3

# Reduce writes to the microSD card, which holds the OS, database, and
# uploaded photos together (see spec: "the card is a consumable").
sudo mount -o remount,noatime /

sudo mkdir -p /etc/systemd/journald.conf.d
cat <<'EOF' | sudo tee /etc/systemd/journald.conf.d/volatile.conf
[Journal]
Storage=volatile
EOF

sudo dphys-swapfile swapoff || true
sudo systemctl disable dphys-swapfile || true

sudo useradd --system --home /srv/homesite --shell /usr/sbin/nologin homesite || true
sudo mkdir -p /srv/homesite/data /srv/homesite/uploads /srv/homesite/content
sudo chown -R homesite:homesite /srv/homesite/data /srv/homesite/uploads
sudo chown -R "$(whoami)":homesite /srv/homesite/content
sudo chmod -R 0775 /srv/homesite/content

sudo systemctl restart systemd-journald

echo "Provisioning complete. Next: copy config.example.yaml to /srv/homesite/config.yaml, fill in real values, and run 'make deploy' from the Mac."
