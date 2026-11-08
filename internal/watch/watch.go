// Package watch schedules serial generations from changing input snapshots.
package watch

import (
	"context"
	"errors"
	"maps"
	"time"
)

// Snapshot identifies input membership and content. Keys are stable absolute
// identities; a zero digest may represent an absent optional input. Observers
// may reuse the map on their next call: Run retains its own copy.
type Snapshot map[string][32]byte

// Options controls polling and trailing-edge change coalescing.
type Options struct {
	Interval time.Duration
	Debounce time.Duration
}

// Run observes inputs until ctx is canceled. The first successful observation
// establishes a baseline without generating; callers perform initial generation
// before Run. Later changes are debounced, then generated serially. Callback
// failures are reported without stopping observation. Ordinary generation failures
// require changed input to retry; an ObservationError preserves unpublished work
// until observation succeeds again. Run returns ctx.Err() on cancellation. A nil
// report discards diagnostics.
func Run(ctx context.Context, opts Options, observe func() (Snapshot, error), generate func(context.Context) error, report func(error)) error {
	if opts.Interval <= 0 {
		return errors.New("watch interval must be positive")
	}
	if opts.Debounce < 0 {
		return errors.New("watch debounce must not be negative")
	}

	var previous Snapshot
	var initialized, pending, observingError bool
	var lastError string
	var due time.Time
	observeInputs := func() bool {
		snapshot, err := observe()
		if ctx.Err() != nil {
			return false
		}
		if err != nil {
			message := err.Error()
			if (!observingError || message != lastError) && report != nil {
				report(err)
			}
			observingError, lastError = true, message
			return false
		}
		observingError, lastError = false, ""
		if !initialized {
			previous, initialized = maps.Clone(snapshot), true
		} else if !maps.Equal(previous, snapshot) {
			previous = maps.Clone(snapshot)
			pending = true
			due = time.Now().Add(opts.Debounce)
		}
		return true
	}

	poll := time.NewTicker(opts.Interval)
	defer poll.Stop()
	debounce := time.NewTimer(opts.Interval)
	debounce.Stop()
	defer debounce.Stop()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		valid := observeInputs()
		if err := ctx.Err(); err != nil {
			return err
		}
		if valid && pending && !time.Now().Before(due) {
			// Ordinary failures consume this change. A failed post-generation
			// observation instead leaves unpublished work pending for recovery.
			pending = false
			err := generate(ctx)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var observationError *ObservationError
			if errors.As(err, &observationError) {
				pending = true
			}
			if err != nil && report != nil {
				report(err)
			}
			// Observe immediately after generation instead of waiting for the
			// next poll, so edits made during a long generation are not lost.
			continue
		}

		var debounceReady <-chan time.Time
		if valid && pending {
			debounce.Reset(time.Until(due))
			debounceReady = debounce.C
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-poll.C:
		case <-debounceReady:
		}
		debounce.Stop()
	}
}
