package model

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
)

type Project struct {
	Types []Type
}

type Type struct {
	Package    string
	Name       string
	Kind       TypeKind
	Visibility Visibility
	Fields     []Field
	Methods    []Method
	Extends    []string
	Implements []string
	Source     string
}

func (t Type) QualifiedName() string {
	if t.Package == "" {
		return t.Name
	}
	return t.Package + "." + t.Name
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
