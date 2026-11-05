package sequence

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSelectCanonicalEntriesAndAmbiguity(t *testing.T) {
	program := Program{Callables: []Callable{
		{ID: "root::p.Service.run(string)", Selector: "p.Service.run(string)", Qualified: "p.Service.run", Name: "run"},
		{ID: "root::p.Service.run(int)", Selector: "p.Service.run(int)", Qualified: "p.Service.run", Name: "run"},
		{ID: "root::p.main()", Selector: "p.main()", Qualified: "p.main", Name: "main"},
	}}
	before := append([]Callable(nil), program.Callables...)
	for _, selector := range []string{"root::p.Service.run(int)", "p.Service.run(int)", "main", "p.main"} {
		entry, err := Select(program, selector)
		if err != nil || entry.ID == "" {
			t.Fatalf("selector %q: entry=%v err=%v", selector, entry.ID, err)
		}
	}
	for _, selector := range []string{"run", "p.Service.run"} {
		_, err := Select(program, selector)
		if !errors.Is(err, ErrAmbiguousEntry) {
			t.Fatalf("selector %q must report ambiguity: %v", selector, err)
		}
		message := err.Error()
		first := strings.Index(message, program.Callables[1].ID)
		second := strings.Index(message, program.Callables[0].ID)
		if first < 0 || second <= first {
			t.Fatalf("canonical candidates must be present in sorted order: %s", message)
		}
	}
	if !reflect.DeepEqual(before, program.Callables) {
		t.Fatal("selection reordered or mutated the analyzed program")
	}
}

func TestSelectMissingAndDuplicateEntries(t *testing.T) {
	for _, selector := range []string{"", "missing", "p.Main.run()"} {
		program := Program{Callables: []Callable{{ID: "z::p.run()", Name: "run"}, {ID: "a::p.run()", Name: "run"}}}
		_, err := Select(program, selector)
		if err == nil || errors.Is(err, ErrAmbiguousEntry) {
			t.Fatalf("missing selector %q must not become an ambiguous success: %v", selector, err)
		}
		if strings.Index(err.Error(), "a::p.run()") > strings.Index(err.Error(), "z::p.run()") {
			t.Fatalf("missing-entry candidates are not sorted: %v", err)
		}
	}
	_, err := Select(Program{Callables: []Callable{{ID: "same"}, {ID: "same"}}}, "same")
	if !errors.Is(err, ErrAmbiguousEntry) {
		t.Fatalf("duplicate canonical IDs must not silently overwrite: %v", err)
	}
}
