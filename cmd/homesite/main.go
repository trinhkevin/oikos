// cmd/homesite/main.go
package main

import (
	"flag"
	"log"
	"net/http"

	"homesite/internal/config"
	"homesite/internal/web"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	srv := web.New(cfg)
	log.Printf("Brivin Household listening on %s", cfg.Site.Listen)
	if err := http.ListenAndServe(cfg.Site.Listen, srv); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
