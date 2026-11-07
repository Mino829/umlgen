package sequencejava

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Mino829/umlgen/internal/sequence"
)

func TestActivityDeadlineClosesInheritedProtocolPipes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX descendant-pipe fixture")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell unavailable")
	}
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, sh, "-c", `sleep 5 & printf '%s' "$!" > "$UMLGEN_CHILD_PID"`)
	cmd.Env = append(os.Environ(), "UMLGEN_CHILD_PID="+pidFile)
	cmd.WaitDelay = 100 * time.Millisecond
	t.Cleanup(func() {
		data, err := os.ReadFile(pidFile)
		if err != nil {
			return
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			t.Fatal(err)
		}
		if child, err := os.FindProcess(pid); err == nil {
			_ = child.Kill()
		}
	})
	started := time.Now()
	_, err = runActivity(cmd, &bytes.Buffer{}, sequence.Program{}, &activityRequest{selector: "entry", maxNodes: 20}, ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lost protocol deadline: %v", err)
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("analysis waited for a descendant's inherited stdout instead of its deadline")
	}
}
