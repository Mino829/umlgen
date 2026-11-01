package explain

import (
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/relations"
)

func TestInferredImplementationShowsEachActualMethodAndCandidateAnchor(t *testing.T) {
	text := Format(Report{
		Analysis: relations.Analysis{Relations: []relations.Relation{{
			From: "Worker", To: "Runner", Kind: relations.Implementation,
			Evidence: []relations.Evidence{
				{Language: "go", Inferred: true, Type: "Runner", Source: "worker.go", Line: 1},
				{Language: "go", Inferred: true, Member: "Run(context.Context) error", Source: "run.go", Line: 2},
				{Language: "go", Inferred: true, Member: "Stop()", Source: "embedded.go", Line: 1},
			},
		}}},
		Sources: map[string]Source{
			"worker.go":   {Content: []byte("type Worker struct { Base }"), Origin: "working tree"},
			"run.go":      {Content: []byte("package worker\nfunc (w *Worker) Run(ctx context.Context) error {\n return nil\n}"), Origin: "working tree"},
			"embedded.go": {Content: []byte("func (b Base) Stop() {}"), Origin: "working tree"},
		},
	})
	requireContains(t, text, "Worker", "run.go:2:", "embedded.go:1:", "Run(context.Context) error", "Stop()", "not compiler-verified", "2 | func (w *Worker) Run", "1 | func (b Base) Stop()")
	requireAbsent(t, text, "Explicit implementation")
}

func TestFailuresUnresolvedAndKnownHiddenAreSeparate(t *testing.T) {
	text := Format(Report{
		Failures: []Failure{{Source: "broken.java", Message: "unexpected token", Origin: "working tree"}},
		Analysis: relations.Analysis{Diagnostics: []relations.ReferenceDiagnostic{
			{From: "Service", Kind: relations.Field, Reference: "Missing<T>", Status: relations.StatusUnresolved, Evidence: relations.Evidence{Source: "service.java", Line: 2, Member: "dependency"}},
			{From: "Service", Kind: relations.Parameter, Reference: "Known", Target: "app.Known", Status: relations.StatusOutsideDiagram, Evidence: relations.Evidence{Source: "service.java", Line: 3, Member: "arg of call(Known)"}},
		}},
		Sources: map[string]Source{"service.java": {Content: []byte("class Service {\n Missing<T> dependency;\n void call(Known arg) {}\n}"), Origin: "working tree"}},
	})
	requireContains(t, text, "Analysis failures:", "broken.java (working tree): unexpected token", "Analysis is incomplete", "Unresolved references:", "Known references outside diagram:", "reference=\"Missing<T>\"", "member=\"dependency\"", "service.java:2", "2 |  Missing<T> dependency;", "3 |  void call(Known arg) {}")
	unresolved := strings.Split(strings.Split(text, "Unresolved references:")[1], "Known references outside diagram:")[0]
	requireContains(t, unresolved, "unresolved", "Missing<T>")
	requireAbsent(t, unresolved, "target=app.Known", "outside-diagram")
	hidden := strings.Split(text, "Known references outside diagram:")[1]
	requireContains(t, hidden, "outside-diagram", "target=app.Known", "member=\"arg of call(Known)\"")
	requireAbsent(t, hidden, "unresolved", "could not be resolved", "unexpected token")
}

func TestCapturedSnapshotsAndPhysicalLineBoundaries(t *testing.T) {
	for _, test := range []struct {
		name    string
		sources map[string]Source
		line    int
		want    []string
		absent  []string
	}{
		{"CRLF and three line limit", map[string]Source{"deleted.go": {Content: []byte("one\r\ntwo\r\nthree\r\nfour\r\nfive\r\n"), Origin: "Git base"}}, 2, []string{"Git base", "2 | two", "3 | three", "4 | four"}, []string{"1 | one", "5 | five", "\r"}},
		{"final line without newline", map[string]Source{"deleted.go": {Content: []byte("one\nlast"), Origin: "Git base"}}, 2, []string{"Git base", "2 | last"}, []string{"3 |", "unavailable"}},
		{"terminal newline has no phantom line", map[string]Source{"deleted.go": {Content: []byte("one\n"), Origin: "working tree"}}, 2, []string{"unavailable", "working tree", "outside", "1 lines"}, []string{"2 |"}},
		{"empty snapshot", map[string]Source{"deleted.go": {Content: []byte{}, Origin: "Git base"}}, 1, []string{"unavailable", "Git base", "0 lines"}, []string{"1 |"}},
		{"missing snapshot", nil, 1, []string{"unavailable", "no captured source snapshot"}, []string{"1 |", "working tree", "Git base"}},
		{"invalid zero line", map[string]Source{"deleted.go": {Content: []byte("one"), Origin: "working tree"}}, 0, []string{"unavailable", "line 0"}, []string{"1 | one"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			text := Format(Report{
				Analysis: relations.Analysis{Relations: []relations.Relation{{From: "A", To: "B", Kind: relations.Field, Label: "field", Multiplicity: "*", Evidence: []relations.Evidence{{Source: "deleted.go", Line: test.line}}}}},
				Sources:  test.sources,
			})
			requireContains(t, text, append(test.want, "A -> B [field")...)
			requireAbsent(t, text, test.absent...)
		})
	}
}

func TestScopeAndFailuresRemainWithoutRelationshipData(t *testing.T) {
	text := Format(Report{
		Scope:    Scope{Targets: []string{"src", "other"}, Excludes: []string{"vendor/**", "**/*_test.go"}, Include: "public", Files: 4, Language: "go"},
		Failures: []Failure{{Source: "old.go", Message: "parse failed", Origin: "Git base"}},
	})
	requireContains(t, text, "Targets: src, other", "Selected files: 4", "Language: go", "Include: public", "Exclude: vendor/**, **/*_test.go", "Analysis failures:", "old.go (Git base): parse failed")
	requireAbsent(t, text, " -> ", "Unresolved references:", "Known references outside diagram:")
}

func requireContains(t *testing.T, text string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(text, fragment) {
			t.Errorf("missing %q in:\n%s", fragment, text)
		}
	}
}

func requireAbsent(t *testing.T, text string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if strings.Contains(text, fragment) {
			t.Errorf("unexpected %q in:\n%s", fragment, text)
		}
	}
}
