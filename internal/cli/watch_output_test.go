package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Mino829/umlgen/internal/watch"
)

func TestWatchRejectsIncompletePairBeforeReplacingSuccessfulOutputs(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	source := filepath.Join(root, "model.puml")
	image := filepath.Join(root, "model.svg")
	incoming := filepath.Join(stage, "model.puml")
	empty := filepath.Join(stage, "model.svg")
	packageCLIWrite(t, source, "old source")
	packageCLIWrite(t, image, "old image")
	packageCLIWrite(t, incoming, "new source")
	packageCLIWrite(t, empty, "")
	err := publishWatchOutputs(context.Background(), stage, []watchOutput{{staged: incoming, final: source}, {staged: empty, final: image}})
	if err == nil || packageCLIRead(t, source) != "old source" || packageCLIRead(t, image) != "old image" {
		t.Fatalf("incomplete pair damaged successful outputs: %v", err)
	}
}

func TestWatchPublicationPreservesExistingPrivatePermissions(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	source := filepath.Join(root, "model.puml")
	incoming := filepath.Join(stage, "model.puml")
	packageCLIWrite(t, source, "old source")
	if err := os.Chmod(source, 0600); err != nil {
		t.Fatal(err)
	}
	packageCLIWrite(t, incoming, "new source")
	before, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := publishWatchOutputs(context.Background(), stage, []watchOutput{{staged: incoming, final: source}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if packageCLIRead(t, source) != "new source" {
		t.Fatal("successful generation did not replace the previous artifact")
	}
	if info.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("published artifact changed existing permissions: before=%v after=%v", before.Mode(), info.Mode())
	}
}

func TestWatchAutomaticEmptyScopeReplacesDeletedTypes(t *testing.T) {
	root := watchTestRoot(t)
	output := filepath.Join(root, "model.puml")
	packageCLIWrite(t, output, "@startuml\nclass DeletedType\n@enduml\n")
	request := watchTestRequest(t, root, "--output", output)
	plan := watchTestPlan(t, request)
	observe := func() (watch.Snapshot, error) { next, err := resolveWatchPlan(request); return next.snapshot, err }
	if err := generateWatch(context.Background(), plan, observe, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if packageCLIRead(t, output) == "@startuml\nclass DeletedType\n@enduml\n" {
		t.Fatal("automatic empty scope retained deleted types")
	}
}

func TestWatchNewArtifactKeepsItsStagedPermissions(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	incoming := filepath.Join(stage, "new.puml")
	output := filepath.Join(root, "new.puml")
	packageCLIWrite(t, incoming, "private diagram")
	if err := os.Chmod(incoming, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(incoming)
	if err != nil {
		t.Fatal(err)
	}
	if err := publishWatchOutputs(context.Background(), stage, []watchOutput{{staged: incoming, final: output}}); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	if after.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("new artifact permissions widened: before=%v after=%v", before.Mode(), after.Mode())
	}
}
