package plantuml

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/umlgen/umlgen/internal/model"
)

type Options struct {
	Title            string
	ShowFields       bool
	ShowMethods      bool
	ShowPrivate      bool
	ShowPublic       bool
	ShowProtected    bool
	ShowPackage      bool
	ShowRelations    bool
	Inheritance      bool
	Implementation   bool
	FieldDependency  bool
	ParamDependency  bool
	ReturnDependency bool
}

func Generate(project model.Project, opts Options) string {
	types := append([]model.Type(nil), project.Types...)
	sort.Slice(types, func(i, j int) bool { return types[i].QualifiedName() < types[j].QualifiedName() })
	aliases, lookup := aliasesFor(types)

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
		fmt.Fprintf(&b, "  %s %q as %s {\n", t.Kind, t.Name, aliases[t.QualifiedName()])
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
		rels := relations(types, aliases, lookup, opts)
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

func aliasesFor(types []model.Type) (map[string]string, map[string]string) {
	aliases := map[string]string{}
	lookup := map[string]string{}
	counts := map[string]int{}
	for _, t := range types {
		counts[t.Name]++
	}
	for _, t := range types {
		alias := safeAlias(t.QualifiedName())
		aliases[t.QualifiedName()] = alias
		lookup[t.QualifiedName()] = alias
		if counts[t.Name] == 1 {
			lookup[t.Name] = alias
		}
		lookup[t.Package+"."+t.Name] = alias
	}
	return aliases, lookup
}

func relations(types []model.Type, aliases, lookup map[string]string, opts Options) []string {
	set := map[string]bool{}
	add := func(from, arrow, rawTarget, pkg string) {
		target := resolve(rawTarget, pkg, lookup)
		if target == "" || target == from {
			return
		}
		set[fmt.Sprintf("  %s %s %s", target, arrow, from)] = true
	}
	dependency := func(from, rawTarget, pkg string) {
		target := resolve(rawTarget, pkg, lookup)
		if target == "" || target == from {
			return
		}
		set[fmt.Sprintf("  %s --> %s", from, target)] = true
	}
	for _, t := range types {
		from := aliases[t.QualifiedName()]
		if opts.Inheritance {
			for _, parent := range t.Extends {
				add(from, "<|--", parent, t.Package)
			}
		}
		if opts.Implementation {
			for _, parent := range t.Implements {
				add(from, "<|..", parent, t.Package)
			}
		}
		if opts.FieldDependency {
			for _, f := range t.Fields {
				for _, ref := range typeReferences(f.Type) {
					dependency(from, ref, t.Package)
				}
			}
		}
		for _, m := range t.Methods {
			if opts.ParamDependency {
				for _, p := range m.Parameters {
					for _, ref := range typeReferences(p.Type) {
						dependency(from, ref, t.Package)
					}
				}
			}
			if opts.ReturnDependency && !m.Constructor {
				for _, ref := range typeReferences(m.ReturnType) {
					dependency(from, ref, t.Package)
				}
			}
		}
	}
	var out []string
	for rel := range set {
		out = append(out, rel)
	}
	sort.Strings(out)
	return out
}

func resolve(name, pkg string, lookup map[string]string) string {
	name = strings.TrimSpace(name)
	if alias := lookup[name]; alias != "" {
		return alias
	}
	if alias := lookup[pkg+"."+name]; alias != "" {
		return alias
	}
	if i := strings.LastIndex(name, "."); i >= 0 {
		return lookup[name[i+1:]]
	}
	return ""
}

func typeReferences(s string) []string {
	var refs []string
	runes := []rune(s)
	for i := 0; i < len(runes); {
		r := runes[i]
		if unicode.IsLetter(r) || r == '_' || r == '$' {
			start := i
			for i < len(runes) {
				r = runes[i]
				if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$' || r == '.') {
					break
				}
				i++
			}
			refs = append(refs, string(runes[start:i]))
		} else {
			i++
		}
	}
	return refs
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
