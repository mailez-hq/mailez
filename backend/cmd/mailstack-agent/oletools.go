package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"

	"mailez/backend/internal/oletools"
)

func runOletools() error {
	addr := os.Getenv("MAILEZ_SCANNER_BINDADDRESS")
	port := os.Getenv("MAILEZ_SCANNER_BINDPORT")
	if port == "" {
		port = "11343"
	}
	minLength := 300
	if v := os.Getenv("MAILEZ_SCANNER_MINLENGTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			minLength = n
		}
	}
	srv := &oletools.Server{MinLength: minLength}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fmt.Fprintf(os.Stderr, "oletools: serving on :%s (min length %d)\n", port, minLength)
	return srv.Serve(ctx, netJoin(addr, port))
}

func netJoin(addr, port string) string {
	if addr == "" {
		return ":" + port
	}
	return addr + ":" + port
}
