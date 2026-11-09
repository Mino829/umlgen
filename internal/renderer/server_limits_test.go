package renderer

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Repeating a small buffer lets the test serve large responses without retaining
// an image-sized allocation in either the handler or the test.
type imageBodyReader struct{}

func (imageBodyReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

func TestServerImageSizeLimit(t *testing.T) {
	const limit = 32 << 20
	for _, chunked := range []bool{false, true} {
		for _, size := range []int64{limit, limit + 1} {
			for _, existing := range []bool{false, true} {
				name := fmt.Sprintf("chunked=%t/size=%d/existing=%t", chunked, size, existing)
				t.Run(name, func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if chunked {
							w.WriteHeader(http.StatusOK)
							w.(http.Flusher).Flush()
						} else {
							w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
						}
						_, _ = io.CopyN(w, imageBodyReader{}, size)
					}))
					defer server.Close()
					puml, image, source := serverLimitInput(t, existing)
					var previousMode os.FileMode
					if existing {
						info, err := os.Stat(image)
						if err != nil {
							t.Fatal(err)
						}
						previousMode = info.Mode().Perm()
					}
					out, err := Render("png", puml, Config{Type: TypeServer, ServerURL: server.URL})
					if size > limit {
						if err == nil || !strings.Contains(err.Error(), "32 MiB") || out != "" {
							t.Fatalf("oversized response: output=%q err=%v", out, err)
						}
						assertFailedServerOutput(t, image, existing)
					} else {
						if err != nil || out != image {
							t.Fatalf("response at limit: output=%q err=%v", out, err)
						}
						info, err := os.Stat(image)
						if err != nil || info.Size() != size {
							t.Fatalf("image size: info=%v err=%v", info, err)
						}
						if existing && info.Mode().Perm() != previousMode {
							t.Fatalf("existing image permissions changed: %v", info.Mode())
						}
						f, err := os.Open(image)
						if err != nil {
							t.Fatal(err)
						}
						defer f.Close()
						var last [1]byte
						if _, err := f.ReadAt(last[:], size-1); err != nil || last[0] != 'x' {
							t.Fatalf("image tail=%q err=%v", last, err)
						}
					}
					assertServerInputRetained(t, puml, source)
					assertNoServerStagingFiles(t, puml)
				})
			}
		}
	}
}

func TestServerRejectsDeclaredOversizeWithoutReadingBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa((32<<20)+1))
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		// The client must reject the declared size instead of waiting for a body.
		<-r.Context().Done()
	}))
	defer server.Close()
	puml, image, source := serverLimitInput(t, true)
	cfg := Defaults()
	cfg.Type, cfg.ServerURL = TypeServer, server.URL
	cfg.Timeout = time.Second
	_, err := Render("png", puml, cfg)
	if err == nil || !strings.Contains(err.Error(), "32 MiB") {
		t.Fatalf("declared oversized response: %v", err)
	}
	assertFailedServerOutput(t, image, true)
	assertServerInputRetained(t, puml, source)
	assertNoServerStagingFiles(t, puml)
}

func TestServerErrorDiagnosticLimit(t *testing.T) {
	const limit = 8 << 10
	for _, chunked := range []bool{false, true} {
		for _, size := range []int{limit, limit + 1} {
			t.Run(fmt.Sprintf("chunked=%t/size=%d", chunked, size), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if !chunked {
						w.Header().Set("Content-Length", strconv.Itoa(size))
					}
					w.WriteHeader(http.StatusBadRequest)
					if chunked {
						w.(http.Flusher).Flush()
					}
					_, _ = io.WriteString(w, "syntax error: "+strings.Repeat("z", size-len("syntax error: ")))
				}))
				defer server.Close()
				puml, image, source := serverLimitInput(t, true)
				_, err := Render("png", puml, Config{Type: TypeServer, ServerURL: server.URL})
				if err == nil || !strings.Contains(err.Error(), "400 Bad Request") || !strings.Contains(err.Error(), "syntax error: ") {
					t.Fatalf("missing server diagnostic: %v", err)
				}
				message := err.Error()
				if strings.Contains(message, "(truncated)") != (size > limit) {
					t.Fatalf("unexpected diagnostic truncation: length=%d", len(message))
				}
				if got, want := strings.Count(message, "z"), limit-len("syntax error: "); got != want {
					t.Fatalf("diagnostic retained %d payload bytes, want %d", got, want)
				}
				assertFailedServerOutput(t, image, true)
				assertServerInputRetained(t, puml, source)
				assertNoServerStagingFiles(t, puml)
			})
		}
	}
}

func TestServerErrorDiagnosticDoesNotWaitForUnboundedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.(http.Flusher).Flush()
		_, _ = io.WriteString(w, "render failed: "+strings.Repeat("x", (8<<10)+1))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	puml, image, source := serverLimitInput(t, false)
	cfg := Defaults()
	cfg.Type, cfg.ServerURL = TypeServer, server.URL
	cfg.Timeout = time.Second
	_, err := Render("png", puml, cfg)
	if err == nil || !strings.Contains(err.Error(), "500 Internal Server Error: render failed: ") || !strings.Contains(err.Error(), "(truncated)") {
		t.Fatalf("unbounded error response: %v", err)
	}
	assertFailedServerOutput(t, image, false)
	assertServerInputRetained(t, puml, source)
	assertNoServerStagingFiles(t, puml)
}

func serverLimitInput(t *testing.T, existing bool) (string, string, string) {
	t.Helper()
	puml := filepath.Join(t.TempDir(), "diagram.puml")
	image := strings.TrimSuffix(puml, ".puml") + ".png"
	source := "@startuml\nclass A\n@enduml\n"
	if err := os.WriteFile(puml, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if existing {
		if err := os.WriteFile(image, []byte("previous image"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return puml, image, source
}

func assertFailedServerOutput(t *testing.T, image string, existing bool) {
	t.Helper()
	data, err := os.ReadFile(image)
	if existing {
		if err != nil || string(data) != "previous image" {
			t.Fatalf("previous image was not preserved: data=%q err=%v", data, err)
		}
	} else if !os.IsNotExist(err) {
		t.Fatalf("failed render left a partial image: size=%d err=%v", len(data), err)
	}
}

func assertServerInputRetained(t *testing.T, puml, source string) {
	t.Helper()
	data, err := os.ReadFile(puml)
	if err != nil || string(data) != source {
		t.Fatalf("input was not preserved: data=%q err=%v", data, err)
	}
}

func assertNoServerStagingFiles(t *testing.T, puml string) {
	t.Helper()
	staged, err := filepath.Glob(filepath.Join(filepath.Dir(puml), ".umlgen-render-*"))
	if err != nil || len(staged) != 0 {
		t.Fatalf("staging files remain: %v err=%v", staged, err)
	}
}
