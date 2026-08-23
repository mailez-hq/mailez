package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mailez/backend/internal/core"
	"mailez/backend/internal/server"
)

// @title mailez API
// @version 0.1.0
// @description Self-hosted mail platform — REST API for the admin console and webmail.
// @BasePath /api/v1
// @securityDefinitions.apikey SessionCookie
// @in cookie
// @name mailez_session
func main() {
	cfg := core.Load()
	srv := server.New(cfg)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.App.Listen(":" + cfg.Port) }()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		log.Fatalf("server: %v", err)
	case sig := <-stop:
		log.Printf("received %s, shutting down", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}
}
