// Package packagegraph projects file-scoped declaration and import dependencies.
// It does not load dependencies, type-check builds, or inspect function bodies.
package packagegraph

import (
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/relations"
)

type DependencySource string

const (
	DeclaredTypes DependencySource = "declared-types"
	Imports       DependencySource = "imports"
	Both          DependencySource = "both"
)

// PackageID is independent of a diagram alias. Context identifies the analysis
// source root (Java) or module/root (Go); callers supply that identity explicitly.
// Equal package names in different contexts remain distinct.
type PackageID struct {
	Language string
	Context  string
	Name     string
}

type PackageKind string

const (
	InternalPackage PackageKind = "internal"
	ExternalPackage PackageKind = "external"
)

type Resolution string

const (
	Resolved   Resolution = "resolved"
	External   Resolution = "external"
	Unresolved Resolution = "unresolved"
	Ambiguous  Resolution = "ambiguous"
)

// Source is a parsed snapshot. Go units must have their PackagePath bound before
// Build; graph construction itself performs no filesystem or network access.
type Source struct {
	Context string
	Unit    *model.SourceUnit
}

type File struct {
	Path        string
	ContentHash string
}

type Package struct {
	ID    PackageID
	Kind  PackageKind
	Files []File
}

// ImportEvidence counts one declaration in one original file, never one copy
// per type. Go dot and blank imports retain their explicit Alias values.
type ImportEvidence struct {
	File   File
	Import model.Import
}

// Dependency keeps two different counting units separate. TypeRelations are
// unique relation records (endpoints/kind/label/multiplicity), each with all its
// declaration evidence. Import counts are len(Imports), not type counts or uses.
type Dependency struct {
	From          PackageID
	To            PackageID
	TypeRelations []relations.Relation
	Imports       []ImportEvidence
}

type Target struct {
	Package PackageID
	Type    string
}

// Diagnostic retains uncertainty without inventing an unresolved package node.
// Import/parser diagnostics use Span; type declarations retain their existing
// Line (their parsers do not yet provide full member byte ranges).
type Diagnostic struct {
	From       PackageID
	Status     Resolution
	Code       string
	Reference  string
	File       File
	Span       model.SourceSpan
	Line       int
	Kind       relations.Kind
	Member     string
	Owner      string
	Candidates []Target
}

type Graph struct {
	Source       DependencySource
	Packages     []Package
	Dependencies []Dependency
	Diagnostics  []Diagnostic
}
