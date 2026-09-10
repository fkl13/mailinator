// Command mailinator implements a Mailinator-clone REST API that
// stores mailboxes and messages entirely in memory.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"
)

type config struct {
	port             int
	evictionInterval time.Duration
	messageTTL       time.Duration
}

// An application holds the dependencies of the server.
type application struct {
	config config
	store  store
	logger *slog.Logger
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
		logger: newLogger(),
	}

	server := http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.port),
		Handler: app.logRequests(app.routes()),
	}

	go app.runEviction(context.Background(), cfg.evictionInterval, cfg.messageTTL)

	err := server.ListenAndServe()
	if err != nil {
		log.Fatal("HTTP server failed to start")
	}
}

func newLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, nil))
}
