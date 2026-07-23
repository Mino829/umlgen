package plantuml

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/relations"
)

type Options struct {
	Title              string
	ShowFields         bool
	ShowMethods        bool
	ShowPrivate        bool
	ShowPublic         bool
	ShowProtected      bool
	ShowPackage        bool
	ShowRelations      bool
	Inheritance        bool
	Implementation     bool
	FieldDependency    bool
	ParamDependency    bool
	ReturnDependency   bool
	ShowRelationLabels bool
}

func Generate(project model.Project, opts Options) string {
	types := append([]model.Type(nil), project.Types...)
	sort.Slice(types, func(i, j int) bool { return types[i].QualifiedName() < types[j].QualifiedName() })
	aliases := aliasesFor(types)

	var b strings.Builder
	b.WriteString("@startuml\n")
	if opts.Title != "" {
		fmt.Fprintf(&b, "title %s\n", escapeTitle(opts.Title))
	}
	if len(types) > 0 {
		b.WriteByte('\n')
	}
	lastPackage := "\x00"
	for _, t := range types {
		if t.Package != lastPackage {
			if lastPackage != "\x00" {
				b.WriteString("}\n\n")
			}
			fmt.Fprintf(&b, "package %q {\n", t.Package)
			lastPackage = t.Package
		}
		declaration, stereotype := typeDeclaration(t.Kind)
		fmt.Fprintf(
			&b, "  %s %q as %s%s%s {\n",
			declaration, t.DisplayName(), aliases[t.QualifiedName()], stereotype, changeColor(t.Change),
		)
		if opts.ShowFields {
			for _, f := range t.Fields {
				if visible(f.Visibility, opts) {
					fmt.Fprintf(&b, "    %s%s: %s\n", visibilitySymbol(f.Visibility), f.Name, f.Type)
				}
			}
		}
		if opts.ShowMethods {
			for _, m := range t.Methods {
				if !visible(m.Visibility, opts) {
					continue
				}
				var params []string
				for _, p := range m.Parameters {
					params = append(params, p.Name+": "+p.Type)
				}
				fmt.Fprintf(&b, "    %s%s(%s)", visibilitySymbol(m.Visibility), m.Name, strings.Join(params, ", "))
				if !m.Constructor && m.ReturnType != "" {
					fmt.Fprintf(&b, ": %s", m.ReturnType)
				}
				b.WriteByte('\n')
			}
		}
		b.WriteString("  }\n")
	}
	if lastPackage != "\x00" {
		b.WriteString("}\n")
	}
	if opts.ShowRelations {
		rels := relationLines(types, aliases, opts)
		if len(rels) > 0 {
			b.WriteByte('\n')
			for _, rel := range rels {
				b.WriteString(rel)
				b.WriteByte('\n')
			}
		}
	}
	b.WriteString("\n@enduml\n")
	return b.String()
}

func changeColor(change model.ChangeKind) string {
	switch change {
	case model.Added:
		return " #palegreen"
	case model.Modified:
		return " #lightyellow"
	case model.Deleted:
		return " #lightcoral"
	default:
		return ""
	}
}

func typeDeclaration(kind model.TypeKind) (string, string) {
	if kind == model.Record {
		return "class", " <<record>>"
	}
	return string(kind), ""
}

func aliasesFor(types []model.Type) map[string]string {
	aliases := map[string]string{}
	for _, t := range types {
		alias := safeAlias(t.QualifiedName())
		aliases[t.QualifiedName()] = alias
	}
	return aliases
}

func relationLines(types []model.Type, aliases map[string]string, opts Options) []string {
	var result []string
	for _, relation := range relations.Build(types) {
		if !relationEnabled(relation.Kind, opts) {
			continue
		}
		from, to := aliases[relation.From], aliases[relation.To]
		if from == "" || to == "" {
			continue
		}
		line := ""
		switch relation.Kind {
		case relations.Inheritance:
			line = fmt.Sprintf("  %s <|-- %s", to, from)
		case relations.Implementation:
			line = fmt.Sprintf("  %s <|.. %s", to, from)
		case relations.Field:
			line = fmt.Sprintf("  %s --> %q %s", from, relation.Multiplicity, to)
		case relations.Parameter, relations.Return:
			line = fmt.Sprintf("  %s ..> %q %s", from, relation.Multiplicity, to)
		}
		if opts.ShowRelationLabels && relation.Label != "" {
			line += " : " + relationLabel(relation)
		}
		result = append(result, line)
	}
	sort.Strings(result)
	return result
}

func relationEnabled(kind relations.Kind, opts Options) bool {
	switch kind {
	case relations.Inheritance:
		return opts.Inheritance
	case relations.Implementation:
		return opts.Implementation
	case relations.Field:
		return opts.FieldDependency
	case relations.Parameter:
		return opts.ParamDependency
	case relations.Return:
		return opts.ReturnDependency
	default:
		return false
	}
}

func relationLabel(relation relations.Relation) string {
	switch relation.Kind {
	case relations.Field:
		return "field " + relation.Label
	case relations.Parameter:
		return "parameter " + relation.Label
	case relations.Return:
		return "returns " + relation.Label
	default:
		return relation.Label
	}
}

func visible(v model.Visibility, opts Options) bool {
	switch v {
	case model.Public:
		return opts.ShowPublic
	case model.Protected:
		return opts.ShowProtected
	case model.Private:
		return opts.ShowPrivate
	default:
		return opts.ShowPackage
	}
}

func visibilitySymbol(v model.Visibility) string {
	switch v {
	case model.Public:
		return "+"
	case model.Protected:
		return "#"
	case model.Private:
		return "-"
	default:
		return "~"
	}
}

func safeAlias(s string) string {
	var b strings.Builder
	b.WriteString("T_")
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func escapeTitle(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", " ")
}
