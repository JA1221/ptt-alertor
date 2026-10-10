// Package jobs runs background checks: new articles of boards, new comments of tracked articles,
// PTT availability and alerts of failures
package jobs

import (
	"context"
	"time"
)

// sleep waits d, returns false when ctx is done first
func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// loop runs round repeatedly, each round takes cycle at least
func loop(ctx context.Context, cycle time.Duration, round func(ctx context.Context)) {
	for ctx.Err() == nil {
		start := time.Now()
		round(ctx)
		if !sleep(ctx, cycle-time.Since(start)) {
			return
		}
	}
}
