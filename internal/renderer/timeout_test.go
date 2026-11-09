package renderer

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

type delayedImageTransport struct{}

func (delayedImageTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	select {
	case <-time.After(31 * time.Second):
		return &http.Response{
			StatusCode: http.StatusOK, ContentLength: -1,
			Body: io.NopCloser(strings.NewReader("<svg xmlns=\"http://www.w3.org/2000/svg\"/>")),
		}, nil
	case <-request.Context().Done():
		return nil, request.Context().Err()
	}
}

func TestZeroTimeoutAllowsRenderingBeyondThirtySeconds(t *testing.T) {
	previous := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: delayedImageTransport{}}
	t.Cleanup(func() { http.DefaultClient = previous })
	for _, timeout := range []time.Duration{0, 30 * time.Second} {
		t.Run(timeout.String(), func(t *testing.T) {
			puml, _, source := serverLimitInput(t, false)
			image := outputPath(puml, "svg")
			synctest.Test(t, func(t *testing.T) {
				started := time.Now()
				_, err := Render("svg", puml, Config{Type: TypeServer, ServerURL: "http://renderer.invalid", Timeout: timeout})
				if timeout == 0 {
					if err != nil {
						t.Fatalf("explicit zero imposed a rendering deadline: %v", err)
					}
					if elapsed := time.Since(started); elapsed <= 30*time.Second {
						t.Fatalf("render did not exercise the previous timeout boundary: %s", elapsed)
					}
				} else if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("positive rendering deadline was not enforced: %v", err)
				}
			})
			if timeout != 0 {
				assertFailedServerOutput(t, image, false)
			}
			assertServerInputRetained(t, puml, source)
		})
	}
}
