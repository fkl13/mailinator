package main

import (
	"context"
	"time"
)

func (app *application) runEviction(ctx context.Context, interval, maxAge time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			count := app.store.Evict(maxAge)
			app.logger.Info("evicted messages", "count", count)
		case <-ctx.Done():
			return
		}
	}
}
