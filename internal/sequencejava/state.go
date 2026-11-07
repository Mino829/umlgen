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
	"github.com/Mino829/umlgen/internal/state"
)

type stateRequest struct {
	selector string
	field    string
	maxNodes int
	analysis state.Analysis
}

// AnalyzeState extracts compiler-bound direct field assignment candidates, never
// inferred transitions or initial states. Selection and CFG construction share
// one attributed snapshot AST; no target code or annotation processor executes.
func AnalyzeState(ctx context.Context, sources []sequence.Source, typeSelector string, field string, maxNodes int, opts Options) (state.Analysis, error) {
	request := &stateRequest{selector: typeSelector, field: field, maxNodes: maxNodes}
	request.analysis = state.Analysis{Language: "java", Backend: "jdk-javac"}
	if maxNodes < 1 {
		return request.analysis, errors.New("state max-nodes must be positive")
	}
	program, err := analyze(ctx, sources, opts, nil, request)
	request.analysis.Language, request.analysis.Backend = program.Language, program.Backend
	request.analysis.Files = program.Files
	if request.analysis.Diagnostics == nil {
		request.analysis.Diagnostics = program.Diagnostics
	}
	if err != nil {
		return request.analysis, err
	}
	for _, graph := range request.analysis.Functions {
		if err := graph.Validate(maxNodes); err != nil {
			return request.analysis, err
		}
	}
	return request.analysis, nil
}

func runState(cmd *exec.Cmd, input *bytes.Buffer, program sequence.Program, request *stateRequest, ctx context.Context) (sequence.Program, error) {
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
	// A direct pipe read can outlive WaitDelay when inherited by descendants.
	stopIO := context.AfterFunc(ctx, func() { stdin.Close(); stdout.Close() })
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
		return fmt.Errorf("trusted Java state compiler protocol: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if _, err = io.Copy(stdin, input); err != nil {
		return program, failure(err)
	}
	reader := &io.LimitedReader{R: stdout, N: (64 << 20) + 1}
	decoder := json.NewDecoder(reader)
	var metadata struct {
		sequence.Program
		Targets []state.Target `json:"targets"`
	}
	if err = decoder.Decode(&metadata); err != nil {
		return program, failure(err)
	}
	program = metadata.Program
	selected, selectionErr := state.SelectTarget(metadata.Targets, request.selector)
	id := selected.ID
	if selectionErr != nil {
		id = ""
	} else {
		request.analysis.Target = selected
	}
	for _, value := range []string{id, request.field} {
		if uint64(len(value)) > uint64(^uint32(0)) {
			return program, failure(errors.New("state selection exceeds framing limits"))
		}
		if err = binary.Write(stdin, binary.BigEndian, uint32(len(value))); err != nil {
			return program, failure(err)
		}
		if _, err = io.WriteString(stdin, value); err != nil {
			return program, failure(err)
		}
	}
	stdin.Close()
	var response struct {
		state.Analysis
		Limit          bool   `json:"limit"`
		SelectionError string `json:"selection_error"`
		Ambiguous      bool   `json:"ambiguous"`
	}
	if err = decoder.Decode(&response); err != nil {
		return program, failure(err)
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return program, failure(errors.New("trailing or oversized Java state protocol data"))
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
		return program, selectionErr
	}
	if response.Target.ID != "" {
		request.analysis.Target = response.Target
	}
	request.analysis.Diagnostics = response.Diagnostics
	if response.SelectionError != "" {
		if response.Ambiguous {
			return program, fmt.Errorf("%w: %w: %s", state.ErrSelection, state.ErrAmbiguous, response.SelectionError)
		}
		return program, fmt.Errorf("%w: %s", state.ErrSelection, response.SelectionError)
	}
	request.analysis.Values, request.analysis.Updates = response.Values, response.Updates
	request.analysis.Functions = response.Functions
	if response.Limit {
		return program, fmt.Errorf("%w: cap %d; increase --max-nodes", activity.ErrLimit, request.maxNodes)
	}
	return program, nil
}
