package sequencejava

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Mino829/umlgen/internal/sequence"
)

func TestActivityInputBoundsAndCancellation(t *testing.T) {
	root := t.TempDir()
	source := sequence.Source{Path: filepath.Join(root, "Sample.java"), Root: root, Content: []byte("class Sample { static void entry() {} }")}
	for _, tc := range []struct {
		name    string
		sources []sequence.Source
		cap     int
		opts    Options
	}{
		{"zero-cap", []sequence.Source{source}, 0, Options{}},
		{"negative-cap", []sequence.Source{source}, -1, Options{}},
		{"release", []sequence.Source{source}, 20, Options{Release: 8}},
		{"duplicate", []sequence.Source{source, source}, 20, Options{}},
		{"relative", []sequence.Source{{Path: "Sample.java", Root: root}}, 20, Options{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := AnalyzeActivity(context.Background(), tc.sources, "entry", tc.cap, tc.opts); err == nil {
				t.Fatal("invalid activity input accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := AnalyzeActivity(ctx, []sequence.Source{source}, "entry", 20, Options{})
	if err == nil {
		t.Fatal("canceled activity succeeded")
	}
	if _, lookErr := exec.LookPath("java"); lookErr == nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("lost context cancellation: %v", err)
	}
}
