// Package activity models explicit, source-backed intraprocedural control flow.
// Edges describe syntactic possibilities, not observed execution or a proof of
// reachability. Calls and implicit exceptional transfers are not expanded.
package activity

import (
	"errors"
	"fmt"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/sequence"
)

var (
	ErrSelection = errors.New("activity function selection failed")
	ErrLimit     = errors.New("activity node limit exceeded")
)

const DefaultMaxNodes = 500

type Node struct {
	ID    int              `json:"id"`
	Kind  string           `json:"kind"`
	Label string           `json:"label"`
	File  string           `json:"file"`
	Span  model.SourceSpan `json:"span"`
}

type Edge struct {
	From  int    `json:"from"`
	To    int    `json:"to"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

// Function contains the canonical declaration identity and span, not sequence
// events. Unsupported nodes are explicit terminal analysis barriers: they must
// never acquire a normal successor that pretends the unmodeled region executed.
type Graph struct {
	Language    string                `json:"language"`
	Backend     string                `json:"backend"`
	Files       []sequence.File       `json:"files"`
	Function    sequence.Callable     `json:"function"`
	Nodes       []Node                `json:"nodes"`
	Edges       []Edge                `json:"edges"`
	Entry       int                   `json:"entry"`
	Exit        int                   `json:"exit"`
	Diagnostics []sequence.Diagnostic `json:"diagnostics"`
}

// Validate refuses invalid/truncated graphs rather than drawing a plausible
// subset after hitting a limit. Synthetic entry/exit nodes count toward the cap.
func (g Graph) Validate(maxNodes int) error {
	if maxNodes < 1 {
		return errors.New("activity max-nodes must be positive")
	}
	if len(g.Nodes) > maxNodes {
		return fmt.Errorf("%w: %d nodes, cap %d; increase --max-nodes", ErrLimit, len(g.Nodes), maxNodes)
	}
	ids := make(map[int]string, len(g.Nodes))
	for _, n := range g.Nodes {
		if n.ID < 0 {
			return fmt.Errorf("negative activity node ID %d", n.ID)
		}
		if _, found := ids[n.ID]; found {
			return fmt.Errorf("duplicate activity node %d", n.ID)
		}
		switch n.Kind {
		case "entry", "exit", "action", "decision", "merge", "return", "throw", "unsupported":
		default:
			return fmt.Errorf("unknown activity node kind %q", n.Kind)
		}
		ids[n.ID] = n.Kind
	}
	if ids[g.Entry] != "entry" || ids[g.Exit] != "exit" || g.Entry == g.Exit {
		return errors.New("activity graph requires distinct entry and exit nodes")
	}
	for _, e := range g.Edges {
		from, found := ids[e.From]
		if !found {
			return fmt.Errorf("activity edge has missing source %d", e.From)
		}
		if _, found := ids[e.To]; !found {
			return fmt.Errorf("activity edge has missing destination %d", e.To)
		}
		if from == "exit" || from == "unsupported" {
			return fmt.Errorf("terminal activity node %d has an outgoing edge", e.From)
		}
		switch e.Kind {
		case "normal", "true", "false", "back", "break", "continue", "return", "throw", "panic", "case", "default", "fallthrough", "goto", "unknown":
		default:
			return fmt.Errorf("unknown activity edge kind %q", e.Kind)
		}
	}
	return nil
}
