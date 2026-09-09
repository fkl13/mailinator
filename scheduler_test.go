package main

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestRunEviction(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		app := application{
			store: NewStore(),
		}
		address := "a@b.com"
		app.store.Create(address)
		msg, err := app.store.AddMessage(address, "x@y.com", "subject", "body")
		if err != nil {
			t.Fatalf("setup: AddMessage failed: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go app.runEviction(ctx, time.Second, time.Minute)

		time.Sleep(90 * time.Second)
		synctest.Wait()

		if _, err := app.store.GetMessage(address, msg.ID); !errors.Is(err, ErrMessageNotFound) {
			t.Fatalf("message not evicted, got err=%v", err)
		}
	})
}
