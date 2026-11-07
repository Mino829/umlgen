package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validMachine = `version: 1
type: Connection
field: phase
initial: closed
states:
  - name: closed
    value: Closed
  - name: open
    value: Open
transitions:
  - from: closed
    to: open
    event: Connect
    guard: accepted
    action: socket.open
`

func TestDefinitionRejectsAmbiguousAndContradictoryContracts(t *testing.T) {
	for _, text := range []string{
		strings.Replace(validMachine, "version: 1", "version: '1'", 1),
		strings.Replace(validMachine, "initial: closed", "initial: missing", 1),
		strings.Replace(validMachine, "name: open", "name: closed", 1),
		strings.Replace(validMachine, "to: open", "to: missing", 1),
		strings.Replace(validMachine, "event: Connect", "event: ''", 1),
		strings.Replace(validMachine, "guard: accepted", "guard: true", 1),
		strings.Replace(validMachine, "field: phase", "fields: phase", 1),
		validMachine + "---\nversion: 1\n",
		validMachine + "  - from: closed\n    to: closed\n    event: Connect\n    guard: accepted\n",
		validMachine + "initial: open\n",
	} {
		path := filepath.Join(t.TempDir(), "machine.yaml")
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadDefinition(path); err == nil {
			t.Errorf("accepted invalid machine:\n%s", text)
		}
	}
}

func TestDefinitionKeepsDisplayTextAndOriginWithoutExecution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "machine.yaml")
	marker := filepath.Join(t.TempDir(), "SHOULD_NOT_EXIST")
	text := strings.Replace(validMachine, "action: socket.open", "action: 'touch "+marker+"'", 1)
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	d, err := LoadDefinition(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.States[0].Line != 6 || d.Transitions[0].Line != 11 {
		t.Fatalf("lost definition source locations: %+v", d)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("display-only action was executed: %v", err)
	}
}
