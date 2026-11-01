package plantuml

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
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
	PackageGrouping    string
	LayoutDirection    string
	SkinParams         map[string]string
}

func (opts Options) ValidateLayout() error {
	switch opts.PackageGrouping {
	case "", "flat", "hierarchical", "none":
	default:
		return fmt.Errorf("invalid package grouping %q: expected flat, hierarchical, or none", opts.PackageGrouping)
	}
	switch opts.LayoutDirection {
	case "", "top-to-bottom", "left-to-right":
	default:
		return fmt.Errorf("invalid layout direction %q: expected top-to-bottom or left-to-right", opts.LayoutDirection)
	}
	keys := make([]string, 0, len(opts.SkinParams))
	for key := range opts.SkinParams {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !validSkinParam(key, opts.SkinParams[key]) {
			return fmt.Errorf("invalid skinparam %q=%q: allowed settings are linetype (ortho/polyline), shadowing (true/false), and classAttributeIconSize (nonnegative integer)", key, opts.SkinParams[key])
		}
	}
	return nil
}

func validSkinParam(key, value string) bool {
	switch key {
	case "linetype":
		return value == "ortho" || value == "polyline"
	case "shadowing":
		return value == "true" || value == "false"
	case "classAttributeIconSize":
		if value == "" {
			return false
		}
		for _, c := range value {
			if c < '0' || c > '9' {
				return false
			}
		}
		_, err := strconv.ParseUint(value, 10, 31)
		return err == nil
	default:
		return false
	}
}

func writePreamble(b *strings.Builder, title string, opts Options) {
	b.WriteString("@startuml\n")
	if title != "" {
		fmt.Fprintf(b, "title %s\n", escapeTitle(title))
	}
	if opts.LayoutDirection == "left-to-right" {
		b.WriteString("left to right direction\n")
	}
	keys := make([]string, 0, len(opts.SkinParams))
	for key, value := range opts.SkinParams {
		// Generate and Placeholder cannot return errors; never emit raw settings,
		// even if a caller omitted ValidateLayout.
		if validSkinParam(key, value) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(b, "skinparam %s %s\n", key, opts.SkinParams[key])
	}
}

// Placeholder renders an empty diff diagram using the same layout as Generate.
func Placeholder(title string, opts Options) string {
	if title == "" {
		title = "Pull request class diagram"
	}
	var b strings.Builder
	writePreamble(&b, title, opts)
	b.WriteString("note \"No displayable changed types in the selected range\" as NoChanges\n@enduml\n")
	return b.String()
}

func Generate(project model.Project, opts Options) string {
	types := append([]model.Type(nil), project.Types...)
	sort.Slice(types, func(i, j int) bool { return types[i].QualifiedName() < types[j].QualifiedName() })
	aliases := aliasesFor(types)

	var b strings.Builder
	writePreamble(&b, opts.Title, opts)
	if len(types) > 0 {
		b.WriteByte('\n')
	}
	switch opts.PackageGrouping {
	case "none":
		for _, t := range types {
			writeType(&b, t, aliases[t.QualifiedName()], "", opts)
		}
	case "hierarchical":
		writeHierarchy(&b, types, aliases, opts)
	default:
		lastPackage := "\x00"
		for _, t := range types {
			if t.Package != lastPackage {
				if lastPackage != "\x00" {
					b.WriteString("}\n\n")
				}
				fmt.Fprintf(&b, "package %q {\n", t.Package)
				lastPackage = t.Package
			}
			writeType(&b, t, aliases[t.QualifiedName()], "  ", opts)
		}
		if lastPackage != "\x00" {
			b.WriteString("}\n")
		}
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

func writeType(b *strings.Builder, t model.Type, alias, indent string, opts Options) {
	declaration, stereotype := typeDeclaration(t)
	fmt.Fprintf(b, "%s%s %q as %s%s%s {\n", indent, declaration, t.DisplayName(), alias, stereotype, changeColor(t.Change))
	if opts.ShowFields {
		for _, value := range t.EnumValues {
			fmt.Fprintf(b, "%s  %s\n", indent, value)
		}
		for _, f := range t.Fields {
			if visible(f.Visibility, opts) {
				fmt.Fprintf(b, "%s  %s%s%s: %s\n", indent, memberModifier(f.Static, false), visibilitySymbol(f.Visibility), f.Name, f.Type)
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
			showAbstract := m.Abstract && t.Kind != model.Interface && t.Kind != model.Annotation
			fmt.Fprintf(b, "%s  %s%s%s(%s)", indent, memberModifier(m.Static, showAbstract), visibilitySymbol(m.Visibility), m.Name, strings.Join(params, ", "))
			if !m.Constructor && m.ReturnType != "" {
				fmt.Fprintf(b, ": %s", m.ReturnType)
			}
			b.WriteByte('\n')
		}
	}
	fmt.Fprintf(b, "%s}\n", indent)
}

type packageNode struct {
	types    []int
	children map[string]*packageNode
}

func writeHierarchy(b *strings.Builder, types []model.Type, aliases map[string]string, opts Options) {
	root := &packageNode{}
	for i := range types {
		t := &types[i]
		node := root
		if t.Package != "" {
			separator := "."
			if strings.Contains(t.Package, "/") || strings.EqualFold(filepath.Ext(t.Source), ".go") {
				separator = "/"
			}
			for _, name := range strings.Split(t.Package, separator) {
				if node.children == nil {
					node.children = make(map[string]*packageNode)
				}
				child := node.children[name]
				if child == nil {
					child = &packageNode{}
					node.children[name] = child
				}
				node = child
			}
		}
		node.types = append(node.types, i)
	}
	// Package aliases use a separate namespace from type aliases. Assigning
	// them in sorted traversal order also distinguishes repeated leaf names.
	nextPackage := 0
	var writeNode func(*packageNode, string)
	writeNode = func(node *packageNode, indent string) {
		for _, i := range node.types {
			writeType(b, types[i], aliases[types[i].QualifiedName()], indent, opts)
		}
		names := make([]string, 0, len(node.children))
		for name := range node.children {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			nextPackage++
			fmt.Fprintf(b, "%spackage %q as P_%d {\n", indent, name, nextPackage)
			writeNode(node.children[name], indent+"  ")
			fmt.Fprintf(b, "%s}\n", indent)
		}
	}
	writeNode(root, "")
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

func typeDeclaration(t model.Type) (string, string) {
	declaration := string(t.Kind)
	var stereotypes []string
	if t.Kind == model.Struct {
		declaration = "class"
		stereotypes = append(stereotypes, "struct")
	}
	if t.Kind == model.Record {
		declaration = "class"
		stereotypes = append(stereotypes, "record")
	}
	if t.Kind == model.Class && t.Abstract {
		declaration = "abstract class"
	}
	if t.Final {
		stereotypes = append(stereotypes, "final")
	}
	if t.Sealed {
		stereotypes = append(stereotypes, "sealed")
	}
	if t.NonSealed {
		stereotypes = append(stereotypes, "non-sealed")
	}
	if len(stereotypes) == 0 {
		return declaration, ""
	}
	return declaration, " <<" + strings.Join(stereotypes, ",") + ">>"
}

func memberModifier(static, abstract bool) string {
	var modifiers []string
	if static {
		modifiers = append(modifiers, "{static}")
	}
	if abstract {
		modifiers = append(modifiers, "{abstract}")
	}
	if len(modifiers) == 0 {
		return ""
	}
	return strings.Join(modifiers, " ") + " "
}

func aliasesFor(types []model.Type) map[string]string {
	aliases := map[string]string{}
	reserved := map[string]bool{}
	var names []string
	for _, t := range types {
		name := t.QualifiedName()
		names = append(names, name)
		reserved[safeAlias(name)] = true
	}
	sort.Strings(names)
	used := map[string]bool{}
	for _, name := range names {
		if _, exists := aliases[name]; exists {
			continue
		}
		base := safeAlias(name)
		alias := base
		if used[alias] {
			for suffix := 2; ; suffix++ {
				alias = fmt.Sprintf("%s_%d", base, suffix)
				if !reserved[alias] && !used[alias] {
					break
				}
			}
		}
		aliases[name] = alias
		used[alias] = true
	}
	return aliases
}

func relationLines(types []model.Type, aliases map[string]string, opts Options) []string {
	var result []string
	for _, relation := range relations.Build(types) {
		if !opts.IncludesRelation(relation.Kind) {
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

// IncludesRelation is the shared selection policy for diagram and explanations.
func (opts Options) IncludesRelation(kind relations.Kind) bool {
	if !opts.ShowRelations {
		return false
	}
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
