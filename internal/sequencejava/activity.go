package sequencejava

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/Mino829/umlgen/internal/activity"
	"github.com/Mino829/umlgen/internal/sequence"
)

type activityRequest struct {
	selector string
	maxNodes int
	graph    activity.Graph
}

// AnalyzeActivity builds one source-backed, intraprocedural graph. The trusted
// compiler retains its AST while Go selects declaration metadata with Select;
// no sequence events, second compilation, target classes or processors are run.
// Calls are atomic actions; implicit exceptions and callee bodies are not expanded.
func AnalyzeActivity(ctx context.Context, sources []sequence.Source, selector string, maxNodes int, opts Options) (activity.Graph, error) {
	request := &activityRequest{selector: selector, maxNodes: maxNodes}
	if maxNodes < 1 {
		return activity.Graph{Language: "java", Backend: "jdk-javac"}, errors.New("activity max-nodes must be positive")
	}
	program, err := analyze(ctx, sources, opts, request)
	request.graph.Language, request.graph.Backend = program.Language, program.Backend
	request.graph.Files = program.Files
	if request.graph.Diagnostics == nil {
		request.graph.Diagnostics = program.Diagnostics
	}
	if err != nil {
		return request.graph, err
	}
	if err = request.graph.Validate(maxNodes); err != nil {
		return request.graph, err
	}
	return request.graph, nil
}

// The activity protocol adds a metadata/selection handshake to the same framed
// snapshot transport. Keeping the compiler process alive preserves the exact AST
// and lets sequence.Select own all selector precedence and ambiguity semantics.
func runActivity(cmd *exec.Cmd, input *bytes.Buffer, program sequence.Program, request *activityRequest, ctx context.Context) (sequence.Program, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return program, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return program, err
	}
	defer stdout.Close()
	stderr := &boundedBuffer{limit: 64 << 10}
	cmd.Stderr = stderr
	if err = cmd.Start(); err != nil {
		stdin.Close()
		return program, fmt.Errorf("start trusted Java compiler helper: %w", err)
	}
	// StdoutPipe is read directly for the handshake. WaitDelay cannot bound
	// these reads before Wait is reached if a descendant retains stdout.
	// Cancellation must interrupt both the protocol read and request write.
	stopIO := context.AfterFunc(ctx, func() {
		stdin.Close()
		stdout.Close()
	})
	defer stopIO()
	finished := false
	defer func() {
		stdin.Close()
		if !finished {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	failure := func(err error) error {
		if !finished {
			cmd.Process.Kill()
			cmd.Wait()
			finished = true
		}
		if ctx.Err() != nil {
			return fmt.Errorf("Java compiler analysis interrupted: %w", ctx.Err())
		}
		return fmt.Errorf("trusted Java activity compiler protocol: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if _, err = io.Copy(stdin, input); err != nil {
		return program, failure(err)
	}
	reader := &io.LimitedReader{R: stdout, N: (64 << 20) + 1}
	decoder := json.NewDecoder(reader)
	if err = decoder.Decode(&program); err != nil {
		return program, failure(err)
	}
	selected, selectionErr := sequence.Select(program, request.selector)
	id := selected.ID
	if selectionErr != nil {
		id = ""
	} else {
		selected.Body = nil
		request.graph.Function = selected
	}
	if err = binary.Write(stdin, binary.BigEndian, uint32(len(id))); err != nil {
		return program, failure(err)
	}
	if _, err = io.WriteString(stdin, id); err != nil {
		return program, failure(err)
	}
	stdin.Close()
	var response struct {
		activity.Graph
		Limit bool `json:"limit"`
	}
	if err = decoder.Decode(&response); err != nil {
		return program, failure(err)
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return program, failure(errors.New("trailing or oversized Java activity protocol data"))
	}
	if reader.N == 0 {
		return program, failure(errors.New("Java compiler model exceeds 64 MiB output limit"))
	}
	err = cmd.Wait()
	finished = true
	if err != nil {
		return program, failure(err)
	}
	if selectionErr != nil {
		return program, fmt.Errorf("%w: %w", activity.ErrSelection, selectionErr)
	}
	request.graph.Diagnostics = response.Diagnostics
	if response.Limit {
		return program, fmt.Errorf("%w: cap %d; increase --max-nodes", activity.ErrLimit, request.maxNodes)
	}
	request.graph.Nodes, request.graph.Edges = response.Nodes, response.Edges
	request.graph.Entry, request.graph.Exit = response.Entry, response.Exit
	return program, nil
}
