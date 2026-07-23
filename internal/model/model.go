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
	Class     TypeKind = "class"
	Interface TypeKind = "interface"
	Enum      TypeKind = "enum"
	Record    TypeKind = "record"
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
	Wildcard bool
	Static   bool
}

type Type struct {
	Package    string
	Name       string
	Enclosing  []string
	Kind       TypeKind
	Visibility Visibility
	Imports    []Import
	Fields     []Field
	Methods    []Method
	Extends    []string
	Implements []string
	Source     string
	Change     ChangeKind
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
	return strings.Join(parts, ".")
}

type Field struct {
	Name       string
	Type       string
	Visibility Visibility
	Static     bool
}

type Method struct {
	Name        string
	ReturnType  string
	Parameters  []Parameter
	Visibility  Visibility
	Constructor bool
	Static      bool
}

type Parameter struct {
	Name string
	Type string
}
