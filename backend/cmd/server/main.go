package main

import (
	"log"

	"mailess/backend/internal/config"
	"mailess/backend/internal/server"
)

func main() {
	cfg := config.Load()
	srv := server.New(cfg)
	log.Fatal(srv.App.Listen(":" + cfg.Port))
}
