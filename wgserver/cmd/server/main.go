package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"wgserver/internal/config"
	"wgserver/internal/db"
	"wgserver/internal/logger"
	"wgserver/internal/server"
)

func main() {
	cfg := config.Load()

	// init loggers directory
	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		log.Fatalf("failed to create log dir: %v", err)
	}

	// init DB
	if err := db.Init(cfg); err != nil {
		log.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	// http server + websocket
	mux := http.NewServeMux()
	hs := server.NewHub(cfg)
	mux.HandleFunc("/ws", hs.HandleWS)

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Connection().Printf("listening on %s", cfg.ListenAddr())
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	// graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	logger.Connection().Println("server shutdown")
}
