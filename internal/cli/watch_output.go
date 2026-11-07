package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mino829/umlgen/internal/plantuml"
	"github.com/Mino829/umlgen/internal/watch"
)

func generateWatch(ctx context.Context, plan watchPlan, observe func() (watch.Snapshot, error), stdout, stderr io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	parent := filepath.Dir(plan.output)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return &watchAttemptError{exitOutput, err}
	}
	stage, err := os.MkdirTemp(parent, ".umlgen-watch-")
	if err != nil {
		return &watchAttemptError{exitOutput, err}
	}
	defer os.RemoveAll(stage)
	staged := filepath.Join(stage, filepath.Base(plan.output))
	var generated, diagnostics bytes.Buffer
	var code int
	var runErr error
	if plan.empty {
		// Planning already validates flags/configuration. Automatic language
		// detection is undefined for zero sources, so do not invoke a child
		// merely to discover that absence.
		if err := writeDiagram(staged, plantuml.GenerateWatchEmpty(plan.title)); err != nil {
			code, runErr = exitOutput, err
		} else {
			renderConfig := plan.renderer
			renderConfig.Context = ctx
			code, runErr = renderDiagram(plan.format, staged, renderConfig, true, &generated, &diagnostics)
			if code == exitOK && (!plan.common.quiet || plan.explain) {
				fmt.Fprintln(&generated, "Watch scope is empty: no matching source files; no previous types retained.")
			}
		}
	} else {
		childArgs := append([]string{"--output", staged}, plan.args...)
		common := plan.common
		common.ctx, common.strictParse = ctx, true
		common.watchTargets, common.watchExcludes = plan.targets, plan.excludes
		if plan.diagram == "package" {
			code, runErr = runPackage(childArgs, common, &generated, &diagnostics)
		} else {
			code, runErr = runClass(childArgs, common, &generated, &diagnostics)
		}
	}
	// Explain and diagnostics stay visible, but staged files are never advertised
	// as successful final artifacts. Only publication prints an update message.
	flush := func() {
		flushWatchText(stdout, generated.String(), stage, parent)
		flushWatchText(stderr, diagnostics.String(), stage, parent)
	}
	if err := ctx.Err(); err != nil {
		flush()
		return err
	}
	if code != exitOK || runErr != nil {
		flush()
		if runErr == nil {
			runErr = fmt.Errorf("diagram generation failed with exit code %d", code)
		}
		return &watchAttemptError{code, runErr}
	}
	current, err := observe()
	if err != nil {
		flush()
		return &watch.ObservationError{Err: err}
	}
	if !maps.Equal(plan.snapshot, current) {
		flush()
		return errors.New("inputs changed during generation; unfinished result discarded")
	}
	outputs := []watchOutput{{staged: staged, final: plan.output}}
	if plan.format == "svg" || plan.format == "png" {
		outputs = append(outputs, watchOutput{staged: watchImagePath(staged, plan.format), final: watchImagePath(plan.output, plan.format)})
	}
	if err := publishWatchOutputs(ctx, stage, outputs); err != nil {
		flush()
		return &watchAttemptError{exitOutput, err}
	}
	flush()
	if !plan.common.quiet {
		for _, output := range outputs {
			fmt.Fprintf(stdout, "Updated %s\n", output.final)
		}
	}
	return nil
}

func watchImagePath(path, format string) string {
	return strings.TrimSuffix(path, filepath.Ext(path)) + "." + format
}
func flushWatchText(w io.Writer, text, stage, parent string) {
	for line := range strings.SplitAfterSeq(text, "\n") {
		if strings.HasPrefix(line, "Generated ") && strings.Contains(line, stage) {
			continue
		}
		io.WriteString(w, strings.ReplaceAll(line, stage, parent))
	}
}

type watchOutput struct {
	staged, final, backup string
	existed               bool
}

// Each replacement is atomic. A multi-file publication rolls back completed
// replacements if a later rename/cancellation fails; it is not an OS-wide atomic
// transaction for concurrent readers of both files.
func publishWatchOutputs(ctx context.Context, stage string, outputs []watchOutput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for i := range outputs {
		output := &outputs[i]
		incoming, err := os.Stat(output.staged)
		if err != nil {
			return err
		}
		if !incoming.Mode().IsRegular() || incoming.Size() == 0 {
			return errors.New("watch generation produced an empty/nonregular artifact")
		}
		info, err := os.Lstat(output.final)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		mode := incoming.Mode().Perm()
		if err == nil {
			if !info.Mode().IsRegular() {
				return errors.New("watch output must be a regular file, not a directory or symbolic link")
			}
			output.existed = true
			mode = info.Mode().Perm()
			output.backup = filepath.Join(stage, fmt.Sprintf("backup-%d", i))
			if err := copyWatchFile(output.final, output.backup, mode); err != nil {
				return err
			}
		}
		if err := os.Chmod(output.staged, mode); err != nil {
			return err
		}
	}
	completed := 0
	for i := range outputs {
		err := ctx.Err()
		if err == nil {
			err = os.Rename(outputs[i].staged, outputs[i].final)
		}
		if err != nil {
			failures := []error{err}
			for j := completed - 1; j >= 0; j-- {
				old := outputs[j]
				var restore error
				if old.existed {
					restore = os.Rename(old.backup, old.final)
				} else {
					restore = os.Remove(old.final)
				}
				if restore != nil {
					failures = append(failures, fmt.Errorf("restore previous watch artifact: %w", restore))
				}
			}
			return errors.Join(failures...)
		}
		completed++
	}
	return nil
}
func copyWatchFile(source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	return errors.Join(copyErr, output.Close())
}
