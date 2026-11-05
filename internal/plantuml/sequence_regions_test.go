package plantuml

import (
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/sequence"
)

func TestSequenceRetainsEmptyExplicitAndUncertainRegions(t *testing.T) {
	view := sequenceDiagramFixture()
	conditional := diagramStep(sequence.Alt, "if condition")
	conditional.Event.Exhaustive = true
	conditional.Children = []sequence.Step{diagramStep(sequence.Return, "return true arm")}
	barrier := diagramStep(sequence.Unsupported, "try handler transfer unknown")
	barrier.Branches = []sequence.StepBranch{{Label: "empty catch"}, {Label: "empty finally"}}
	view.Steps = []sequence.Step{conditional, barrier}
	text, err := GenerateSequence(view, SequenceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"else otherwise", "empty catch", "empty finally"} {
		if !strings.Contains(text, required) {
			t.Errorf("empty source region disappeared: %q\n%s", required, text)
		}
	}
}
