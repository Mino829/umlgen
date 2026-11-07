package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWatchInitialRenderRecoversAfterIdenticalConfigurationRestored(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var first sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		first.Do(func() {
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
			}
		})
		io.WriteString(w, "<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>")
	}))
	defer server.Close()
	root := watchTestRoot(t)
	source := filepath.Join(root, "src", "app.go")
	output := filepath.Join(root, "model.puml")
	cfg := filepath.Join(root, "settings.yaml")
	packageCLIWrite(t, source, "package app\ntype InitialRecovery struct{}\n")
	valid := "language: go\nsource: [src]\noutput:\n  file: model.svg\n  format: svg\nrenderer:\n  type: server\n  server_url: " + server.URL + "\n"
	packageCLIWrite(t, cfg, valid)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var log watchLog
	result := make(chan packageCLIResult, 1)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		code, err := runWatch([]string{"--config", cfg, "--interval", "5ms", "--debounce", "10ms", "--quiet"}, commonOptions{ctx: ctx}, io.Discard, &log)
		result <- packageCLIResult{code: code, err: err}
	}()
	defer func() {
		cancel()
		select {
		case <-stopped:
		case <-time.After(10 * time.Second):
			t.Error("initial recovery watcher did not stop")
		}
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("initial render did not start")
	}
	packageCLIWrite(t, cfg, "source: [\n")
	close(release)
	awaitWatch(t, func() bool {
		select {
		case ended := <-result:
			t.Fatalf("transient observation error terminated watcher: %+v", ended)
		default:
		}
		return log.contains(cfg)
	})
	packageCLIWrite(t, cfg, valid)
	awaitWatch(t, func() bool { data, _ := os.ReadFile(output); return strings.Contains(string(data), "InitialRecovery") })
	cancel()
	select {
	case ended := <-result:
		if ended.code != exitOK || ended.err != nil {
			t.Fatalf("recovered watcher failed: %+v", ended)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("recovered watcher did not stop")
	}
}
