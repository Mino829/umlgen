package sequencejava

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Mino829/umlgen/internal/sequence"
	"github.com/Mino829/umlgen/internal/state"
)

func TestStateDuplicateQualifiedTypesAcrossRootsNeverMerge(t *testing.T) {
	requireJDK(t)
	base := t.TempDir()
	first, second := filepath.Join(base, "a"), filepath.Join(base, "b")
	sources := []sequence.Source{
		{Root: first, Path: filepath.Join(first, "Twin.java"), Content: []byte("class Twin { int state; void first() { state=1; } }")},
		{Root: second, Path: filepath.Join(second, "Twin.java"), Content: []byte("class Twin { int state; void second() { state=2; } }")},
	}
	a, err := AnalyzeState(context.Background(), sources, "Twin", "state", 500, Options{})
	if !errors.Is(err, state.ErrAmbiguous) || !errors.Is(err, state.ErrSelection) || len(a.Files) != 2 || len(a.Diagnostics) == 0 {
		t.Fatalf("same qualified name lost root ambiguity/compiler diagnostics: %+v %v", a, err)
	}
	// javac reports duplicate declarations rather than creating independent
	// language contexts. A canonical selector must never silently union them.
	for _, root := range []string{first, second} {
		a, err := AnalyzeState(context.Background(), sources, root+"::Twin", "state", 500, Options{})
		if err != nil && !errors.Is(err, state.ErrSelection) {
			t.Fatal(err)
		}
		if len(a.Files) != 2 || len(a.Diagnostics) == 0 {
			t.Fatalf("lost duplicate-source evidence: %+v", a)
		}
		if err == nil {
			if a.Target.ID != root+"::Twin" || len(a.Updates) != 1 || a.Updates[0].File != filepath.Join(root, "Twin.java") {
				t.Fatalf("canonical selection merged compiler-damaged roots: %+v", a)
			}
		}
	}
}
