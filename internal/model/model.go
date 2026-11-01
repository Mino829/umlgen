package model

import "strings"

type Visibility string

const (
	Public    Visibility = "public"
	Protected Visibility = "protected"
	Private   Visibility = "private"
	Package   Visibility = "package"
)

type TypeKind string

const (
	Class      TypeKind = "class"
	Struct     TypeKind = "struct"
	Interface  TypeKind = "interface"
	Annotation TypeKind = "annotation"
	Enum       TypeKind = "enum"
	Record     TypeKind = "record"
)

type Project struct {
	Types []Type
}

type ChangeKind string

const (
	Unchanged ChangeKind = ""
	Added     ChangeKind = "added"
	Modified  ChangeKind = "modified"
	Deleted   ChangeKind = "deleted"
)

type Import struct {
	Name     string
	Alias    string
	Wildcard bool
	Static   bool
}

type Type struct {
	Package        string
	Name           string
	Enclosing      []string
	Kind           TypeKind
	Visibility     Visibility
	TypeParameters string
	Abstract       bool
	Final          bool
	Sealed         bool
	NonSealed      bool
	Imports        []Import
	EnumValues     []string
	Fields         []Field
	Methods        []Method
	Extends        []TypeReference
	Implements     []TypeReference
	Source         string `json:"-"`
	Line           int
	Change         ChangeKind
	// ImplementationMethods is derived by Go finalization only when explaining.
	ImplementationMethods []ImplementationMethod `json:"-"`
}

func (t Type) QualifiedName() string {
	parts := append([]string{}, t.Enclosing...)
	parts = append(parts, t.Name)
	if t.Package != "" {
		parts = append([]string{t.Package}, parts...)
	}
	return strings.Join(parts, ".")
}

func (t Type) DisplayName() string {
	parts := append([]string{}, t.Enclosing...)
	parts = append(parts, t.Name)
	return strings.Join(parts, ".") + t.TypeParameters
}

// TypeReference locates an explicit parent or an inferred Go implementation.
type TypeReference struct {
	Name     string
	Source   string `json:"-"`
	Line     int
	Inferred bool
}

// ImplementationMethod locates a method used by syntactic interface matching.
type ImplementationMethod struct {
	Interface string
	Signature string
	Source    string
	Line      int
}

type Field struct {
	Name       string
	Type       string
	Visibility Visibility
	Static     bool
	Source     string `json:"-"`
	Line       int
}

type Method struct {
	Name            string
	ReturnType      string
	Parameters      []Parameter
	Visibility      Visibility
	Constructor     bool
	Static          bool
	Abstract        bool
	PointerReceiver bool
	Source          string `json:"-"`
	Line            int
}

type Parameter struct {
	Name string
	Type string
	Line int
}
