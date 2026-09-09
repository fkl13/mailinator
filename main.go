package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
)

type config struct {
	port             int
	evictionInterval time.Duration
	messageTTL       time.Duration
}

type application struct {
	config config
	store  store
}

func main() {
	var cfg config

	flag.IntVar(&cfg.port, "port", 8080, "API server port")
	flag.DurationVar(&cfg.evictionInterval, "eviction-interval", 5*time.Minute, "Message eviction interval")
	flag.DurationVar(&cfg.messageTTL, "message-ttl", 2*time.Hour, "Message time to live")
	flag.Parse()

	app := application{
		config: cfg,
		store:  NewStore(),
	}

	server := http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.port),
		Handler: app.routes(),
	}

	go app.runEviction(context.Background(), cfg.evictionInterval, cfg.messageTTL)

	err := server.ListenAndServe()
	if err != nil {
		log.Fatal("HTTP server failed to start")
	}
}
