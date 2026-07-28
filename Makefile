.PHONY: dev test build deploy

dev:
	templ generate --watch &
	go run ./cmd/homesite -config config.local.yaml

test:
	go test ./...

build:
	templ generate
	GOOS=linux GOARCH=arm64 go build -o homesite ./cmd/homesite

deploy: build
	scp homesite apollo@apollo:/srv/homesite/homesite.new
	ssh apollo@apollo 'mv /srv/homesite/homesite.new /srv/homesite/homesite'
	scp -r deploy apollo@apollo:/srv/homesite/deploy
	ssh apollo@apollo 'sudo systemctl restart homesite'
	@echo "Binary and deploy/ (including backup.sh, at the path homesite-backup.service expects) deployed; service restarted."
	@echo "First-time setup only: see docs/RUNBOOK.md steps 2-6 for provisioning, config, and systemd units."
