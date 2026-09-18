// Command mailinator implements a Mailinator-clone REST API that
// stores mailboxes and messages entirely in memory.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/emersion/go-smtp"
	"golang.org/x/sync/errgroup"
)

type config struct {
	port             int
	smtpPort         int
	evictionInterval time.Duration
	messageTTL       time.Duration
	smtpDomain       string
}

// An application holds the dependencies of the server.
type application struct {
	config config
	store  *store
	logger *slog.Logger
}

func run(cfg config) error {
	app := application{
		config: cfg,
		store:  newStore(),
		logger: newLogger(),
	}

	httpServer := http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.port),
		Handler: app.logRequests(app.routes()),
	}

	smtpServer := NewSMTPServer(cfg.smtpDomain, cfg.smtpPort, app.store, app.logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	group, groupCtx := errgroup.WithContext(ctx)

	// HTTP server
	group.Go(func() error {
		app.logger.Info("starting HTTP server", "port", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	})

	// SMTP server
	group.Go(func() error {
		app.logger.Info("starting SMTP server", "port", smtpServer.Addr)
		if err := smtpServer.ListenAndServe(); err != nil && !errors.Is(err, smtp.ErrServerClosed) {
			return fmt.Errorf("smtp server: %w", err)
		}
		return nil
	})

	// Eviction job
	group.Go(func() error {
		app.runEviction(groupCtx, cfg.evictionInterval, cfg.messageTTL)
		return nil
	})

	// when ctx is canceled, shut job and servers down
	group.Go(func() error {
		<-groupCtx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			app.logger.Error("http shutdown error", "error", err)
		}

		if err := smtpServer.Shutdown(shutdownCtx); err != nil {
			app.logger.Error("smtp shutdown error", "error", err)
			smtpServer.Close()
		}

		return nil
	})

	return group.Wait()
}

func main() {
	var cfg config

	flag.IntVar(&cfg.port, "port", 8080, "API server port")
	flag.DurationVar(&cfg.evictionInterval, "eviction-interval", 5*time.Minute, "Message eviction interval")
	flag.DurationVar(&cfg.messageTTL, "message-ttl", 2*time.Hour, "Message time to live")
	flag.IntVar(&cfg.smtpPort, "smtp-port", 2525, "SMTP server port")
	flag.StringVar(&cfg.smtpDomain, "smtp-domain", "localhost", "SMTP server domain")
	flag.Parse()

	if err := run(cfg); err != nil {
		log.Fatal(err)
	}
}

func newLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, nil))
}
