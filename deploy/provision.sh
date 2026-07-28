#!/usr/bin/env bash
# deploy/provision.sh — run once on a fresh Raspberry Pi OS Lite (Trixie) install.
set -euo pipefail

sudo apt update
sudo apt install -y imagemagick restic sqlite3

# The spec's sole stated reason for using ImageMagick over Go's native
# image decoders is HEIC/HEIF support (iPhone photos) — but Debian's
# HEIF delegate doesn't always ship in the base imagemagick metapackage.
# Fail loudly here, at provisioning time, rather than mysteriously
# during a party when iPhone uploads fail and Android uploads don't.
if ! magick -list format 2>/dev/null | grep -qi heic; then
	echo "ERROR: ImageMagick has no HEIC delegate. Install the HEIF support package for your distro before continuing." >&2
	exit 1
fi

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
sudo usermod -aG homesite "$(whoami)"
sudo mkdir -p /srv/homesite/data /srv/homesite/uploads /srv/homesite/content
sudo chown -R homesite:homesite /srv/homesite/data /srv/homesite/uploads
sudo chown -R "$(whoami)":homesite /srv/homesite/content
sudo chmod -R 0775 /srv/homesite/content

# /srv/homesite itself is left root:root by useradd — group-writable so
# the login user (assumed to be in the homesite group, added above) can
# scp the binary and deploy/ directory there via `make deploy`, without
# which every fresh-Pi deploy gets Permission denied.
sudo chown root:homesite /srv/homesite
sudo chmod 0775 /srv/homesite

sudo systemctl restart systemd-journald

echo "Provisioning complete. Next: copy config.example.yaml to /srv/homesite/config.yaml, fill in real values, and run 'make deploy' from the Mac."
