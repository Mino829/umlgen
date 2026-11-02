package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
)

// SourcePosition is physical: Offset is zero-based, Line and Column are
// one-based, and Column counts bytes rather than Unicode or UTF-16 characters.
type SourcePosition struct {
	Offset int
	Line   int
	Column int
}

// SourceSpan is a half-open byte range within its owning SourceUnit.
type SourceSpan struct {
	Start SourcePosition
	End   SourcePosition
}

type PackageDeclaration struct {
	Name string
	Span SourceSpan
}

// SourceDiagnostic records a parser limitation or syntax failure, not a
// conclusion about whether dependencies exist.
type SourceDiagnostic struct {
	Code string
	Span SourceSpan
}

// SourceUnit owns a file's syntax, including files without type declarations.
// Path and PackagePath are rebound per run; source bytes and ASTs are not kept.
// Types share this file's Imports for class relation analysis. Imports are
// persisted once at file scope, not once for every type.
type SourceUnit struct {
	Language    string
	Path        string `json:"-"`
	ContentHash string
	Span        SourceSpan
	Package     PackageDeclaration
	Imports     []Import
	Types       []Type
	Diagnostics []SourceDiagnostic
	PackagePath string `json:"-"`
	// PackageContext is the transient Go module root, or the containing directory
	// when no module exists. The CLI carries Java source-root context separately.
	PackageContext string `json:"-"`
}

func NewSourceUnit(language, path string, source []byte) *SourceUnit {
	hash := sha256.Sum256(source)
	end := SourcePosition{
		Offset: len(source),
		Line:   bytes.Count(source, []byte{'\n'}) + 1,
		Column: len(source) - bytes.LastIndexByte(source, '\n'),
	}
	return &SourceUnit{
		Language: language, Path: path, ContentHash: hex.EncodeToString(hash[:]),
		Span: SourceSpan{Start: SourcePosition{Line: 1, Column: 1}, End: end},
	}
}

// RebindPath restores transient declaration paths and file-scoped imports after
// a content cache hit at another path. Package context is applied separately.
func (u *SourceUnit) RebindPath(path string) {
	u.Path = path
	for i := range u.Types {
		t := &u.Types[i]
		t.Source = path
		t.Imports = u.Imports
		for j := range t.Fields {
			t.Fields[j].Source = path
		}
		for j := range t.Methods {
			t.Methods[j].Source = path
		}
		for j := range t.Extends {
			t.Extends[j].Source = path
		}
		for j := range t.Implements {
			t.Implements[j].Source = path
		}
	}
}
