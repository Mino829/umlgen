// Package sequencejava analyzes captured Java sources with the installed JDK compiler.
package sequencejava

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Mino829/umlgen/internal/sequence"
)

// Options selects the Java language/platform release. Zero means 17.
type Options struct{ Release int }

//go:embed CompilerBridge.java
var helper []byte

// Analyze parses and attributes only the supplied source snapshots. It never
// builds the project, loads its processors, or executes its code. Compiler
// diagnostics are returned in Program; process/protocol failures are errors.
//
// The installed JDK must support source launch and compiler release 17 or later.
// Only platform classes and selected snapshots can bind; no application classpath
// or sourcepath is loaded. Virtual/interface calls retain open-world alternatives.
// Initializers and lambdas are separate regions, not implicit invocations. Unknown
// expressions, unsupported transfers and try/catch/finally have explicit barriers.
// Spans address the captured UTF-8 bytes, not files currently on disk.
//
// The embedded bridge protocol is UMLSEQJ1, big-endian uint32 release and source
// count, then uint32-length-prefixed UTF-8 path, root and content for each source.
// Its sole stdout value is a JSON sequence.Program. Calls without a context
// deadline are bounded to 30 seconds; an explicit caller deadline takes precedence.
func Analyze(ctx context.Context, sources []sequence.Source, opts Options) (sequence.Program, error) {
	return analyze(ctx, sources, opts, nil)
}

func analyze(ctx context.Context, sources []sequence.Source, opts Options, request *activityRequest) (sequence.Program, error) {
	program := sequence.Program{Language: "java", Backend: "jdk-javac", Files: []sequence.File{}, Callables: []sequence.Callable{}, Diagnostics: []sequence.Diagnostic{}}
	if opts.Release == 0 {
		opts.Release = 17
	}
	if opts.Release < 17 {
		return program, fmt.Errorf("Java sequence analysis requires release >=17")
	}
	seen := make(map[string]bool, len(sources))
	for _, source := range sources {
		if !filepath.IsAbs(source.Path) || !filepath.IsAbs(source.Root) {
			return program, fmt.Errorf("Java source path and root must be absolute: %q", source.Path)
		}
		if seen[source.Path] {
			return program, fmt.Errorf("duplicate Java source path: %q", source.Path)
		}
		seen[source.Path] = true
		if !utf8.Valid(source.Content) || !utf8.ValidString(source.Path) || !utf8.ValidString(source.Root) {
			return program, fmt.Errorf("Java source must be UTF-8: %q", source.Path)
		}
		hash := sha256.Sum256(source.Content)
		program.Files = append(program.Files, sequence.File{Path: source.Path, Hash: hex.EncodeToString(hash[:])})
	}
	java, err := exec.LookPath("java")
	if err != nil {
		return program, fmt.Errorf("Java sequence analysis requires an installed JDK >=17: %w", err)
	}
	dir, err := os.MkdirTemp("", "umlgen-sequence-java-")
	if err != nil {
		return program, fmt.Errorf("create trusted Java helper directory: %w", err)
	}
	defer os.RemoveAll(dir)
	empty := filepath.Join(dir, "empty")
	if err = os.Mkdir(empty, 0700); err != nil {
		return program, err
	}
	path := filepath.Join(dir, "CompilerBridge.java")
	if err = os.WriteFile(path, helper, 0600); err != nil {
		return program, err
	}
	var input bytes.Buffer
	if request == nil {
		input.WriteString("UMLSEQJ1")
	} else {
		input.WriteString("UMLACTJ1")
	}
	if uint64(opts.Release) > uint64(^uint32(0)) || uint64(len(sources)) > uint64(^uint32(0)) {
		return program, errors.New("Java analysis input exceeds framing limits")
	}
	binary.Write(&input, binary.BigEndian, uint32(opts.Release))
	binary.Write(&input, binary.BigEndian, uint32(len(sources)))
	if request != nil {
		binary.Write(&input, binary.BigEndian, int64(request.maxNodes))
	}
	for _, source := range sources {
		for _, value := range [][]byte{[]byte(source.Path), []byte(source.Root), source.Content} {
			if uint64(len(value)) > uint64(^uint32(0)) {
				return program, errors.New("Java source exceeds framing limits")
			}
			binary.Write(&input, binary.BigEndian, uint32(len(value)))
			input.Write(value)
		}
	}
	// Honor an explicit caller deadline; bound otherwise unbounded API calls.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, java, "-Xmx256m", "-XX:MaxMetaspaceSize=128m", "--class-path", empty, "--source", "17", path, empty)
	cmd.Dir = dir
	cmd.WaitDelay = 100 * time.Millisecond
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		switch strings.ToUpper(name) {
		case "JAVA_TOOL_OPTIONS", "JDK_JAVA_OPTIONS", "_JAVA_OPTIONS", "CLASSPATH", "JAVA_OPTIONS", "JDK_JAVAC_OPTIONS":
			continue
		}
		cmd.Env = append(cmd.Env, item)
	}
	if request == nil {
		cmd.Stdin = &input
	} else {
		return runActivity(cmd, &input, program, request, ctx)
	}
	stdout := &boundedBuffer{limit: 64 << 20}
	stderr := &boundedBuffer{limit: 64 << 10}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err = cmd.Run()
	if ctx.Err() != nil {
		return program, fmt.Errorf("Java compiler analysis interrupted: %w", ctx.Err())
	}
	if err != nil {
		return program, fmt.Errorf("trusted Java compiler helper failed (JDK >=17 required): %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if stdout.overflow {
		return program, errors.New("Java compiler model exceeds 64 MiB output limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	if err = decoder.Decode(&program); err != nil {
		return program, fmt.Errorf("decode Java compiler model: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return program, errors.New("Java compiler helper emitted trailing protocol data")
	}
	return program, nil
}

type boundedBuffer struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - b.Len()
	if len(p) > remaining {
		b.overflow = true
		p = p[:remaining]
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}
