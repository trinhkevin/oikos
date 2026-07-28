.PHONY: dev build deploy test

dev:
	templ generate --watch &
	go run ./cmd/homesite -config config.local.yaml

test:
	go test ./...

build:
	templ generate
	GOOS=linux GOARCH=arm64 go build -o homesite ./cmd/homesite

deploy: build
	scp homesite pi@192.168.1.50:/srv/homesite/homesite
	ssh pi@192.168.1.50 'sudo systemctl restart homesite'
