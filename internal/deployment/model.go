// Package deployment describes declared topology, never observed runtime state.
package deployment

import "errors"

var ErrInput = errors.New("invalid deployment input")

type Origin struct {
	File     string
	Hash     string
	Format   string
	Document int
	Line     int
}

type ID struct{ Kind, Namespace, Name string }

type Port struct {
	Scope     string // host, service or node: a declaration, not observed reachability.
	Published string
	Target    string
	Protocol  string
	Origin    Origin
}

type Node struct {
	ID       ID
	Name     string
	Origin   Origin
	Ports    []Port
	Replicas *int
}

type Edge struct {
	From, To ID
	Kind     string
	Detail   string
	Origin   Origin
}

type Diagnostic struct {
	Code    string
	Message string // Fixed descriptions only: never raw YAML or ignored values.
	Origin  Origin
}

type Graph struct {
	Sources     []Origin
	Nodes       []Node
	Edges       []Edge
	Diagnostics []Diagnostic
	Incomplete  bool
}
