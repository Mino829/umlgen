package cli

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func stateFixture(t *testing.T, language, root string) string {
	t.Helper()
	closed, open := "Closed", "Open"
	if language == "go" {
		packageCLIWrite(t, filepath.Join(root, "go.mod"), "module example.test/state\n\ngo 1.25\n")
		packageCLIWrite(t, filepath.Join(root, "connection.go"), `package connection
type Phase int
const (Closed Phase = iota; Open)
type Connection struct { phase Phase }
func(c *Connection) Connect(accepted bool) { if accepted { c.phase = Open } }
func(c *Connection) Close() { c.phase = Closed }
func(c *Connection) Restore(next Phase) { c.phase = next }
`)
	} else {
		closed, open = "CLOSED", "OPEN"
		packageCLIWrite(t, filepath.Join(root, "Connection.java"), `package sample;
class Connection {
 enum Phase { CLOSED, OPEN }
 Phase phase = Phase.CLOSED;
 void connect(boolean accepted) { if(accepted) { phase = Phase.OPEN; } }
 void close() { phase = Phase.CLOSED; }
 void restore(Phase next) { phase = next; }
}`)
	}
	definition := filepath.Join(root, "machine.yaml")
	packageCLIWrite(t, definition, fmt.Sprintf("version: 1\ntype: Connection\nfield: phase\ninitial: closed\nstates:\n  - name: closed\n    value: %s\n  - name: open\n    value: %s\ntransitions:\n  - from: closed\n    to: open\n    event: Connect\n    guard: accepted\n    action: socket.open\n  - from: open\n    to: closed\n    event: Close\n", closed, open))
	return definition
}

func TestStateCLISeparatesConfiguredTransitionsFromSourceWrites(t *testing.T) {
	for _, language := range []string{"go", "java"} {
		t.Run(language, func(t *testing.T) {
			if language == "java" {
				requireSequenceJDK(t)
			}
			root := t.TempDir()
			definition := stateFixture(t, language, root)
			output := filepath.Join(t.TempDir(), "state.puml")
			args := []string{"state", root, "--language", language, "--state-config", definition, "--quiet", "--explain", "-o", output}
			result := packageCLISuccess(t, args...)
			for _, evidence := range []string{"Configured initial: closed", "previous state UNKNOWN", "accepted", "restore", "CFG nodes", "Definition SHA256:"} {
				if !strings.Contains(strings.ToLower(result.stdout), strings.ToLower(evidence)) {
					t.Errorf("missing state evidence %q:\n%s", evidence, result.stdout)
				}
			}
			diagram := packageCLIRead(t, output)
			solid := regexp.MustCompile(`(?m)^ST_\d+ --> ST_\d+ :`).FindAllString(diagram, -1)
			if len(solid) != 2 || !strings.Contains(diagram, "Unknown previous state") || !strings.Contains(diagram, "Unmapped candidate") {
				t.Fatalf("source writes changed configured transitions or lost unknown candidates:\n%s", diagram)
			}
			for _, flags := range [][]string{{"--state-field", "missing"}, {"--type", "Missing"}, {"--max-nodes", "2"}, {"--max-nodes", "0"}, {"--no-cache"}} {
				r := packageCLIRun(append(args, flags...)...)
				if r.code != exitArgs || r.err == nil || packageCLIRead(t, output) != diagram {
					t.Fatalf("%v did not preserve diagram on an error: %+v", flags, r)
				}
			}
			bad := strings.Replace(packageCLIRead(t, definition), "initial: closed", "initial: undeclared", 1)
			packageCLIWrite(t, definition, bad)
			r := packageCLIRun(args...)
			if r.code != exitArgs || r.err == nil || packageCLIRead(t, output) != diagram {
				t.Fatalf("invalid explicit machine overwrote output: %+v", r)
			}
		})
	}
}

func TestStateCLIConfigRelativeDefinitionAndPartialSourceErrors(t *testing.T) {
	root := t.TempDir()
	stateFixture(t, "go", root)
	configPath := filepath.Join(root, ".umlgen.yaml")
	packageCLIWrite(t, configPath, "language: go\nsource: [.]\noutput:\n  file: diagrams/state.puml\ndiagrams:\n  state:\n    state_config: machine.yaml\n    field: missing\n")
	args := []string{"state", "--config", configPath, "--state-field", "phase", "--quiet", "--explain"}
	packageCLISuccess(t, args...)
	output := filepath.Join(root, "diagrams", "state.puml")
	packageCLIWrite(t, filepath.Join(root, "broken.go"), "package connection\nfunc Broken( {\n")
	r := packageCLIRun(args...)
	if r.code != exitParse || r.err == nil || !strings.Contains(r.stderr, "parse") || !strings.Contains(packageCLIRead(t, output), "@enduml") {
		t.Fatalf("partial source failure erased machine or claimed completeness: %+v", r)
	}
}
