package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type watchLog struct {
	mu   sync.Mutex
	text strings.Builder
}

func (w *watchLog) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.text.Write(p)
}
func (w *watchLog) contains(s string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Contains(w.text.String(), s)
}
func awaitWatch(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("watch did not reach the expected source/output state")
		case <-tick.C:
		}
	}
}

func TestWatchPreservesSuccessfulDiagramThroughPartialParseAndDeletion(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "app.go")
	broken := filepath.Join(root, "broken.go")
	output := filepath.Join(root, "model.puml")
	packageCLIWrite(t, source, "package app\ntype Original struct{}\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var logs watchLog
	finished := make(chan packageCLIResult, 1)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		code, err := runWatch([]string{"class", root, "--language", "go", "--interval", "5ms", "--debounce", "15ms", "--quiet", "-o", output}, commonOptions{ctx: ctx}, io.Discard, &logs)
		finished <- packageCLIResult{code: code, err: err}
	}()
	defer func() {
		cancel()
		select {
		case <-stopped:
		case <-time.After(10 * time.Second):
			t.Error("watch did not stop after cancellation")
		}
	}()
	readOutput := func() string { content, _ := os.ReadFile(output); return string(content) }
	awaitWatch(t, func() bool { return strings.Contains(readOutput(), "Original") })
	previous := readOutput()
	packageCLIWrite(t, source, "package app\ntype Updated struct{}\n")
	packageCLIWrite(t, broken, "package app\nfunc Broken( {\n")
	awaitWatch(t, func() bool { return logs.contains(broken) })
	if readOutput() != previous {
		t.Fatal("partial parse replaced the last successful diagram")
	}
	packageCLIWrite(t, broken, "package app\ntype Repaired struct{}\n")
	awaitWatch(t, func() bool {
		out := readOutput()
		return strings.Contains(out, "Updated") && strings.Contains(out, "Repaired") && !strings.Contains(out, "Original")
	})
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(broken); err != nil {
		t.Fatal(err)
	}
	awaitWatch(t, func() bool {
		out := readOutput()
		return !strings.Contains(out, "Updated") && !strings.Contains(out, "Repaired") && strings.Contains(out, "@enduml")
	})
	cancel()
	select {
	case result := <-finished:
		if result.code != exitOK || result.err != nil {
			t.Fatalf("cancel did not stop cleanly: %+v", result)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("watch remained running")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".umlgen-watch-") {
			t.Fatal("watch leaked unfinished staging")
		}
	}
}
