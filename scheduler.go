package main

import (
	"context"
	"log"
	"time"
)

func (app *application) runEviction(ctx context.Context, interval, maxAge time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			count := app.store.Evict(maxAge)
			log.Printf("Evicted %d messages", count)
		case <-ctx.Done():
			return
		}
	}
}
