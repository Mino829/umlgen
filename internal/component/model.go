// Package component projects source package dependencies onto explicitly
// configured architecture boundaries. It never infers runtime service ownership.
package component

import (
	"errors"

	"github.com/Mino829/umlgen/internal/packagegraph"
)

var ErrBoundary = errors.New("invalid or ambiguous component boundary")

type Selector struct {
	Language      string `yaml:"language"`
	PackagePrefix string `yaml:"package_prefix"`
	SourceRoot    string `yaml:"source_root"`
	GoModule      string `yaml:"go_module"`
	Line          int    `yaml:"-"`
}

// Conditions within one selector are AND; selectors within a boundary are OR.
// SourceRoot is an exact physical analysis-context identity, not a directory glob.
// GoModule is an exact captured go.mod module path, never a Java/build module.
type Boundary struct {
	Name     string     `yaml:"name"`
	Match    []Selector `yaml:"match"`
	Provides []string   `yaml:"provides"`
	Requires []string   `yaml:"requires"`
	Line     int        `yaml:"-"`
}

type Definition struct {
	Version    int        `yaml:"version"`
	Components []Boundary `yaml:"components"`
	Source     string     `yaml:"-"`
	Hash       string     `yaml:"-"`
}

// Module contains captured Go module metadata for one graph context. Its name
// must come from the existing lexical module reader, not package-name guessing.
type Module struct {
	Context string
	Name    string
	File    string
	Hash    string
}

type ID struct {
	Kind    string                 // component, unassigned, external
	Name    string                 // explicit component name only
	Package packagegraph.PackageID // non-component identity; context remains distinct
}

type Node struct {
	ID       ID
	Name     string
	Boundary *Boundary // immutable definition entry, nil for unmapped/external packages
	Packages []packagegraph.Package
	Internal []packagegraph.Dependency
}

// Originals retains original package endpoints and each original relation/import
// record, including its source locations and hash. Counts must keep type-relation
// records, evidence locations and import declarations as distinct counting units.
type Dependency struct {
	From      ID
	To        ID
	Originals []packagegraph.Dependency
}

type Diagnostic struct {
	Code      string
	Message   string
	Component string
	Package   *packagegraph.PackageID
	Line      int // boundary YAML line when applicable
}

type Options struct{ ShowExternal bool }

type Graph struct {
	Definition   Definition
	Source       packagegraph.Graph // complete parsed scope, never mutated
	Modules      []Module
	Nodes        []Node
	Dependencies []Dependency
	Hidden       []packagegraph.Dependency // external-display filtering only
	Diagnostics  []Diagnostic
}
