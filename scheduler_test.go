package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"
)

func TestRunEviction(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		app := application{
			store:  newStore(),
			logger: slog.New(slog.DiscardHandler),
		}
		address := "a@b.com"
		app.store.Create(address)
		msg, err := app.store.AddMessage(address, "x@y.com", "subject", "body")
		if err != nil {
			t.Fatalf("setup: AddMessage failed: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		mailboxTTL := time.Hour
		messageTTL := time.Minute
		go app.runEviction(ctx, time.Second, mailboxTTL, messageTTL)

		// Advance past messageTTL but well short of mailboxTTL.
		time.Sleep(messageTTL + 10*time.Second)
		synctest.Wait()

		if _, err := app.store.GetMessage(address, msg.ID); !errors.Is(err, ErrMessageNotFound) {
			t.Fatalf("message not evicted, got err=%v", err)
		}
		if !app.store.Exists(address) {
			t.Fatalf("mailbox evicted too early")
		}

		// Advance past mailboxTTL.
		time.Sleep(mailboxTTL)
		synctest.Wait()

		if app.store.Exists(address) {
			t.Fatalf("mailbox not evicted")
		}
	})
}
