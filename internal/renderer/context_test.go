package renderer

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCallerCancellationStopsActiveServerRender(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
	defer server.Close()
	defer server.CloseClientConnections()
	path := filepath.Join(t.TempDir(), "diagram.puml")
	if err := os.WriteFile(path, []byte("@startuml\nclass A\n@enduml\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := Render("svg", path, Config{Type: TypeServer, ServerURL: server.URL, Timeout: time.Hour, Context: ctx})
		finished <- err
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("renderer did not make the configured request")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("caller cancellation lost: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("renderer ignored caller cancellation and waited for its long timeout")
	}
}
