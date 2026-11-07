package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type State struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
	Line  int    `yaml:"-"`
}

type Transition struct {
	From   string `yaml:"from"`
	To     string `yaml:"to"`
	Event  string `yaml:"event"`
	Guard  string `yaml:"guard"`
	Action string `yaml:"action"`
	Line   int    `yaml:"-"`
}

// Definition is an explicit contract, never executable code or an inferred
// machine. Type/Field may be supplied by CLI/config; initial/state references
// must already be valid when loading the machine.
type Definition struct {
	Version     int          `yaml:"version"`
	Type        string       `yaml:"type"`
	Field       string       `yaml:"field"`
	Initial     string       `yaml:"initial"`
	States      []State      `yaml:"states"`
	Transitions []Transition `yaml:"transitions"`
	Source      string       `yaml:"-"`
	Hash        string       `yaml:"-"`
}

func LoadDefinition(path string) (Definition, error) {
	var d Definition
	content, err := os.ReadFile(path)
	if err != nil {
		return d, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	var node yaml.Node
	if err = decoder.Decode(&node); err != nil {
		return d, fmt.Errorf("invalid state definition %s: %w", path, err)
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return d, errors.New("state definition must contain exactly one YAML document")
	}
	if err = node.Decode(&d); err != nil {
		return d, err
	}
	if len(node.Content) != 1 {
		return d, errors.New("state definition must be a mapping")
	}
	root := definitionNode(node.Content[0])
	if err = definitionMapping(root, map[string]string{"version": "!!int", "type": "!!str", "field": "!!str", "initial": "!!str", "states": "!!seq", "transitions": "!!seq"}); err != nil {
		return d, err
	}
	stateFields := map[string]string{"name": "!!str", "value": "!!str"}
	transitionFields := map[string]string{"from": "!!str", "to": "!!str", "event": "!!str", "guard": "!!str", "action": "!!str"}
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i].Value, definitionNode(root.Content[i+1])
		if key != "states" && key != "transitions" {
			continue
		}
		fields := stateFields
		if key == "transitions" {
			fields = transitionFields
		}
		for index, item := range value.Content {
			if err = definitionMapping(definitionNode(item), fields); err != nil {
				return d, err
			}
			if key == "states" {
				d.States[index].Line = item.Line
			} else {
				d.Transitions[index].Line = item.Line
			}
		}
	}
	d.Source, err = filepath.Abs(path)
	if err != nil {
		return d, err
	}
	hash := sha256.Sum256(content)
	d.Hash = hex.EncodeToString(hash[:])
	return d, d.Validate()
}

func definitionNode(node *yaml.Node) *yaml.Node {
	for node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	return node
}

func definitionMapping(node *yaml.Node, fields map[string]string) error {
	if node.Kind != yaml.MappingNode {
		return errors.New("state definition and its entries must be mappings")
	}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], definitionNode(node.Content[i+1])
		want, exists := fields[key.Value]
		if !exists || key.Tag != "!!str" {
			return fmt.Errorf("unknown state definition key %q at line %d", key.Value, key.Line)
		}
		if value.Tag != want || (want == "!!seq" && value.Kind != yaml.SequenceNode) || (want != "!!seq" && value.Kind != yaml.ScalarNode) {
			return fmt.Errorf("state definition %s at line %d must have type %s", key.Value, key.Line, want)
		}
	}
	return nil
}

func (d Definition) Validate() error {
	if d.Version != 1 {
		return errors.New("state definition requires version: 1")
	}
	if len(d.States) == 0 {
		return errors.New("state definition requires at least one explicitly named state")
	}
	names := make(map[string]bool, len(d.States))
	for _, s := range d.States {
		if strings.TrimSpace(s.Name) == "" {
			return errors.New("state name must not be empty")
		}
		if names[s.Name] {
			return fmt.Errorf("duplicate state name %q", s.Name)
		}
		names[s.Name] = true
	}
	if !names[d.Initial] {
		return fmt.Errorf("initial state %q is not explicitly declared", d.Initial)
	}
	type eventKey struct{ from, event, guard string }
	events := make(map[eventKey]Transition, len(d.Transitions))
	for _, t := range d.Transitions {
		if !names[t.From] || !names[t.To] {
			return fmt.Errorf("transition %q -> %q refers to an undeclared state", t.From, t.To)
		}
		if strings.TrimSpace(t.Event) == "" {
			return errors.New("configured transition requires a non-empty event")
		}
		key := eventKey{t.From, t.Event, t.Guard}
		if _, exists := events[key]; exists {
			return fmt.Errorf("duplicate/conflicting configured transition for %q / %q / %q", t.From, t.Event, t.Guard)
		}
		events[key] = t
	}
	return nil
}
