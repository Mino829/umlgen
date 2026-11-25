package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWatchSnapshotInputCanBeDeletedWhileOpen(t *testing.T) {
	segment := strings.Repeat("long", 32)
	path := filepath.Join(t.TempDir(), segment, segment, "入力.go")
	const source = "package project\ntype Before struct{}\n"
	packageCLIWrite(t, path, source)
	reader, err := openWatchInput(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := os.Remove(path); err != nil {
		t.Fatalf("snapshot reader prevented deletion: %v", err)
	}
	if got, err := io.ReadAll(reader); err != nil || string(got) != source {
		t.Fatalf("snapshot of deleted input = %q, %v; want %q", got, err, source)
	}
}
