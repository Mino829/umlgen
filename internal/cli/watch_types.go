package cli

import (
	"time"

	"github.com/Mino829/umlgen/internal/renderer"
	"github.com/Mino829/umlgen/internal/watch"
)

type watchRequest struct {
	diagram            string
	args               []string // Normalized diagram flags/target; watch-only flags removed.
	common             commonOptions
	interval, debounce time.Duration
}

type watchPlan struct {
	diagram               string
	args                  []string // Execution flags/target; original output removed so staging can prepend its override.
	common                commonOptions
	output, format, title string
	renderer              renderer.Config
	snapshot              watch.Snapshot
	targets, excludes     []string
	empty                 bool
	explain               bool
	missing               bool // An explicitly selected root is absent; initial start rejects it.
}

type watchAttemptError struct {
	code int
	err  error
}

func (e *watchAttemptError) Error() string { return e.err.Error() }
func (e *watchAttemptError) Unwrap() error { return e.err }
