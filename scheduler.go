package main

import (
	"context"
	"time"
)

func (app *application) runEviction(ctx context.Context, interval, mailboxTTL, messageTTL time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			count := app.store.Evict(mailboxTTL, messageTTL)
			app.logger.Info("evicted messages", "count", count)
		case <-ctx.Done():
			return
		}
	}
}
