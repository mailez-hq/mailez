package main

import (
	"log"

	"mailez/backend/internal/config"
	"mailez/backend/internal/server"
)

func main() {
	cfg := config.Load()
	srv := server.New(cfg)
	log.Fatal(srv.App.Listen(":" + cfg.Port))
}
