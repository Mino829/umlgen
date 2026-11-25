package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchLockedImageCancellationRestoresPreviousPair(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	source := filepath.Join(root, "model.puml")
	image := filepath.Join(root, "model.svg")
	stagedSource := filepath.Join(stage, "model.puml")
	stagedImage := filepath.Join(stage, "model.svg")
	packageCLIWrite(t, source, "old source")
	packageCLIWrite(t, image, "old image")
	packageCLIWrite(t, stagedSource, "new source")
	packageCLIWrite(t, stagedImage, "new image")

	// Go omits FILE_SHARE_DELETE when opening files for reading on Windows.
	reader, err := os.Open(image)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 65*time.Millisecond)
	defer cancel()
	err = publishWatchOutputs(ctx, stage, []watchOutput{
		{staged: stagedSource, final: source}, {staged: stagedImage, final: image},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("locked image did not wait for cancellation: %v", err)
	}
	if got := packageCLIRead(t, source); got != "old source" {
		t.Errorf("previous source was not restored: %q", got)
	}
	if got := packageCLIRead(t, image); got != "old image" {
		t.Errorf("previous image was not preserved: %q", got)
	}
}
