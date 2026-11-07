package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/Mino829/umlgen/internal/watch"
)

func runWatch(args []string, inherited commonOptions, stdout, stderr io.Writer) (int, error) {
	request, help, err := parseWatchRequest(args, inherited)
	if help {
		printWatchHelp(stdout)
		return exitOK, nil
	}
	if err != nil {
		return watchError(err)
	}
	parent := inherited.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt)
	defer stop()
	initial, err := resolveWatchPlan(request)
	if err != nil {
		return watchError(err)
	}
	if initial.missing {
		return exitError, errors.New("watch target does not exist; start with an existing source scope")
	}
	current := initial
	refresh := func() (watch.Snapshot, error) {
		next, err := resolveWatchPlan(request)
		if err != nil {
			return nil, err
		}
		current = next
		return next.snapshot, nil
	}
	generate := func(ctx context.Context) error { return generateWatch(ctx, current, refresh, stdout, stderr) }
	var initialObservationFailure bool
	if err := generate(ctx); err != nil {
		if ctx.Err() != nil {
			return exitOK, nil
		}
		var observation *watch.ObservationError
		initialObservationFailure = errors.As(err, &observation)
		var attempt *watchAttemptError
		if !initialObservationFailure && errors.As(err, &attempt) && attempt.code == exitArgs {
			return attempt.code, attempt.err
		}
		fmt.Fprintf(stderr, "Watch: previous successful output preserved: %v\n", err)
	}
	// The initial generation rechecks its inputs before publication. Retain its
	// baseline even if another edit arrives before the polling loop starts.
	baseline := initial.snapshot
	if initialObservationFailure {
		// Nothing was published for this snapshot. A valid observation must
		// schedule it even when restored config has identical bytes.
		baseline = nil
	}
	first := true
	observe := func() (watch.Snapshot, error) {
		if first {
			first = false
			return baseline, nil
		}
		return refresh()
	}
	if !request.common.quiet {
		fmt.Fprintln(stdout, "Watching source/configuration changes; Ctrl+C stops safely.")
	}
	err = watch.Run(ctx, watch.Options{Interval: request.interval, Debounce: request.debounce}, observe, generate, func(err error) { fmt.Fprintf(stderr, "Watch: previous successful output preserved: %v\n", err) })
	if ctx.Err() != nil {
		if !request.common.quiet {
			fmt.Fprintln(stdout, "Watch stopped.")
		}
		return exitOK, nil
	}
	if err != nil {
		return watchError(err)
	}
	return exitOK, nil
}

func watchError(err error) (int, error) {
	var attempt *watchAttemptError
	if errors.As(err, &attempt) {
		return attempt.code, attempt.err
	}
	return exitError, err
}

func printWatchHelp(w io.Writer) {
	fmt.Fprint(w, `Regenerate class/package diagrams after source or configuration changes.

Usage:
  umlgen watch [class|package] [target] [diagram flags] [watch flags]

Watch flags:
      --interval duration  content-snapshot polling interval (default 500ms, >0)
      --debounce duration  quiet period after the latest change (default 250ms, >=0)

The default diagram is class. All normal class/package flags, configuration
precedence and syntax cache settings apply; see 'umlgen help class/package'.
Java and Go sources, relevant module/language markers and shared configuration
are re-observed. Adds/deletes/renames and configuration scope/output changes apply.
Complete generations are staged before publication. Parse/config/render failures
keep BOTH the previous successful PlantUML and image; fixes resume on input changes.
Empty source scope produces an explicit empty diagram. Generated files and staging
directories do not retrigger generation. Generations never overlap or run hooks.
Ctrl+C cancels rendering, discards unfinished staging and exits successfully.
Other diagram kinds, Git diff, GUI/browser refresh and arbitrary commands are not
watch targets. Polling reads selected input contents; no target source is executed.
`)
}
