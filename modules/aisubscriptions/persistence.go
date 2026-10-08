package aisubscriptions

import (
	"context"
	"os"
	"time"
)

// Keep atomic replacement, including the previous file, while Windows readers
// briefly hold handles without delete sharing. Never remove the destination.
func replaceFile(ctx context.Context, from, to string) error {
	delay := 10 * time.Millisecond
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := os.Rename(from, to)
		if err == nil {
			return nil
		}
		if !transientReplacement(err) || attempt >= 8 {
			return err
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		delay = min(delay*2, 200*time.Millisecond)
	}
}
