package relations

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/Mino829/umlgen/internal/model"
)

type Kind string

const (
	Inheritance    Kind = "inheritance"
	Implementation Kind = "implementation"
	Field          Kind = "field"
	Parameter      Kind = "parameter"
	Return         Kind = "return"
)

type Relation struct {
	From         string
	To           string
	Kind         Kind
	Label        string
	Multiplicity string
	Evidence     []Evidence
}

// Evidence identifies a declaration responsible for a resolved relationship.
type Evidence struct {
	Member   string
	Type     string
	Source   string
	Line     int
	Inferred bool
}

type Index struct {
	types     []model.Type
	qualified map[string]int
}

func NewIndex(types []model.Type) *Index {
	index := &Index{
		types:     types,
		qualified: make(map[string]int, len(types)),
	}
	for i, t := range types {
		index.qualified[t.QualifiedName()] = i
	}
	return index
}

func (i *Index) Resolve(owner model.Type, reference string) (string, bool) {
	reference = Base(reference)
	if reference == "" || isBuiltin(reference) {
		return "", false
	}
	if _, ok := i.qualified[reference]; ok {
		return reference, true
	}

	// Nested and enclosing types take precedence over imports.
	for depth := len(owner.Enclosing); depth >= 0; depth-- {
		parts := []string{}
		if owner.Package != "" {
			parts = append(parts, owner.Package)
		}
		parts = append(parts, owner.Enclosing[:depth]...)
		parts = append(parts, reference)
		if candidate := strings.Join(parts, "."); i.has(candidate) {
			return candidate, true
		}
	}

	for _, imported := range owner.Imports {
		if imported.Static || imported.Wildcard {
			continue
		}
		if imported.Alias == "." {
			if candidate := imported.Name + "." + reference; i.has(candidate) {
				return candidate, true
			}
			continue
		}
		if imported.Alias == "_" {
			continue
		}
		if qualifier, name, ok := strings.Cut(reference, "."); ok {
			alias := imported.Alias
			if alias == "" {
				alias = imported.Name
				if slash := strings.LastIndex(alias, "/"); slash >= 0 {
					alias = alias[slash+1:]
				} else if dot := strings.LastIndex(alias, "."); dot >= 0 {
					alias = alias[dot+1:]
				}
			}
			if alias == qualifier {
				if candidate := imported.Name + "." + name; i.has(candidate) {
					return candidate, true
				}
			}
		}
		if !strings.Contains(reference, ".") &&
			(imported.Name == reference || strings.HasSuffix(imported.Name, "."+reference)) {
			if i.has(imported.Name) {
				return imported.Name, true
			}
		}
	}
	for _, imported := range owner.Imports {
		if imported.Static || !imported.Wildcard {
			continue
		}
		if candidate := imported.Name + "." + reference; i.has(candidate) {
			return candidate, true
		}
	}

	if owner.Package != "" {
		if candidate := owner.Package + "." + reference; i.has(candidate) {
			return candidate, true
		}
	}
	return "", false
}

func (i *Index) has(name string) bool {
	_, ok := i.qualified[name]
	return ok
}

func (i *Index) Type(name string) (model.Type, bool) {
	position, ok := i.qualified[name]
	if !ok {
		return model.Type{}, false
	}
	return i.types[position], true
}

func Build(types []model.Type) []Relation {
	return build(types, false)
}

// BuildWithEvidence preserves every distinct declaration behind each edge.
// Ordinary diagram generation avoids allocating this additional metadata.
func BuildWithEvidence(types []model.Type) []Relation {
	return build(types, true)
}

func build(types []model.Type, explain bool) []Relation {
	index := NewIndex(types)
	unique := map[string]Relation{}
	store := func(r Relation, evidence Evidence) {
		key := relationKey(r)
		if explain {
			previous := unique[key]
			r.Evidence = previous.Evidence
			for _, existing := range r.Evidence {
				if existing == evidence {
					return
				}
			}
			r.Evidence = append(r.Evidence, evidence)
		}
		unique[key] = r
	}
	add := func(owner model.Type, raw string, kind Kind, label, multiplicity string, evidence Evidence) {
		for _, ref := range References(raw) {
			target, ok := index.Resolve(owner, ref)
			if !ok || target == owner.QualifiedName() {
				continue
			}
			store(Relation{
				From: owner.QualifiedName(), To: target, Kind: kind,
				Label: label, Multiplicity: multiplicity,
			}, evidence)
		}
	}
	for _, t := range types {
		for _, parents := range []struct {
			refs []model.TypeReference
			kind Kind
		}{{t.Extends, Inheritance}, {t.Implements, Implementation}} {
			for _, parent := range parents.refs {
				if target, ok := index.Resolve(t, parent.Name); ok && target != t.QualifiedName() {
					store(Relation{From: t.QualifiedName(), To: target, Kind: parents.kind}, Evidence{
						Type: parent.Name, Source: parent.Source, Line: parent.Line, Inferred: parent.Inferred,
					})
				}
			}
		}
		for _, field := range t.Fields {
			add(t, field.Type, Field, field.Name, Multiplicity(field.Type), Evidence{
				Member: field.Name, Type: field.Type, Source: field.Source, Line: field.Line,
			})
		}
		for _, method := range t.Methods {
			var member string
			if explain {
				var signature strings.Builder
				signature.WriteString(method.Name)
				signature.WriteByte('(')
				for i, parameter := range method.Parameters {
					if i > 0 {
						signature.WriteString(", ")
					}
					signature.WriteString(parameter.Type)
				}
				signature.WriteByte(')')
				member = signature.String()
			}
			for _, parameter := range method.Parameters {
				parameterMember := ""
				if explain {
					parameterMember = parameter.Name + " of " + member
				}
				add(t, parameter.Type, Parameter, method.Name+"("+parameter.Name+")", Multiplicity(parameter.Type), Evidence{
					Member: parameterMember, Type: parameter.Type, Source: method.Source, Line: parameter.Line,
				})
			}
			if !method.Constructor {
				add(t, method.ReturnType, Return, method.Name, Multiplicity(method.ReturnType), Evidence{
					Member: member, Type: method.ReturnType, Source: method.Source, Line: method.Line,
				})
			}
		}
	}
	result := make([]Relation, 0, len(unique))
	for _, relation := range unique {
		if explain {
			sort.Slice(relation.Evidence, func(a, b int) bool {
				left, right := relation.Evidence[a], relation.Evidence[b]
				if left.Source != right.Source {
					return left.Source < right.Source
				}
				if left.Line != right.Line {
					return left.Line < right.Line
				}
				if left.Member != right.Member {
					return left.Member < right.Member
				}
				if left.Type != right.Type {
					return left.Type < right.Type
				}
				return !left.Inferred && right.Inferred
			})
		}
		result = append(result, relation)
	}
	sort.Slice(result, func(a, b int) bool {
		left, right := result[a], result[b]
		return relationKey(left) < relationKey(right)
	})
	return result
}

func relationKey(r Relation) string {
	return strings.Join([]string{r.From, r.To, string(r.Kind), r.Label, r.Multiplicity}, "\x00")
}

// Explain formats resolved edges and their declaration evidence as plain text.
func Explain(relations []Relation) string {
	var b strings.Builder
	b.WriteString("Relationship explanations:\n")
	if len(relations) == 0 {
		b.WriteString("  No relationships are displayed.\n")
	}
	for _, relation := range relations {
		fmt.Fprintf(&b, "%s -> %s [%s", relation.From, relation.To, relation.Kind)
		if relation.Label != "" {
			fmt.Fprintf(&b, ", label=%q", relation.Label)
		}
		if relation.Multiplicity != "" {
			fmt.Fprintf(&b, ", multiplicity=%s", relation.Multiplicity)
		}
		b.WriteString("]\n")
		for _, evidence := range relation.Evidence {
			source := evidence.Source
			if source == "" {
				source = "<unknown>"
			}
			fmt.Fprintf(&b, "  %s", source)
			if evidence.Line > 0 {
				fmt.Fprintf(&b, ":%d", evidence.Line)
			}
			b.WriteString(": ")
			declaredType := strings.Join(strings.Fields(evidence.Type), " ")
			switch relation.Kind {
			case Inheritance:
				fmt.Fprintf(&b, "extends/embeds %s", declaredType)
			case Implementation:
				if evidence.Inferred {
					fmt.Fprintf(&b, "inferred implementation: method set satisfies %s", declaredType)
				} else {
					fmt.Fprintf(&b, "implements %s", declaredType)
				}
			case Field:
				fmt.Fprintf(&b, "field %s has type %s", evidence.Member, declaredType)
			case Parameter:
				fmt.Fprintf(&b, "parameter %s has type %s", evidence.Member, declaredType)
			case Return:
				fmt.Fprintf(&b, "return of %s has type %s", evidence.Member, declaredType)
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func References(text string) []string {
	runes := []rune(text)
	var result []string
	for pos := 0; pos < len(runes); {
		if !unicode.IsLetter(runes[pos]) && runes[pos] != '_' && runes[pos] != '$' {
			pos++
			continue
		}
		start := pos
		for pos < len(runes) && (unicode.IsLetter(runes[pos]) || unicode.IsDigit(runes[pos]) ||
			runes[pos] == '_' || runes[pos] == '$' || runes[pos] == '.') {
			pos++
		}
		word := string(runes[start:pos])
		if !typeKeyword(word) {
			result = append(result, word)
		}
	}
	return result
}

func Base(text string) string {
	text = strings.TrimSpace(text)
	for {
		switch {
		case strings.HasPrefix(text, "*"):
			text = strings.TrimSpace(strings.TrimPrefix(text, "*"))
		case strings.HasPrefix(text, "[]"):
			text = strings.TrimSpace(strings.TrimPrefix(text, "[]"))
		case strings.HasPrefix(text, "chan "):
			text = strings.TrimSpace(strings.TrimPrefix(text, "chan "))
		case strings.HasPrefix(text, "<-chan "):
			text = strings.TrimSpace(strings.TrimPrefix(text, "<-chan "))
		default:
			goto normalized
		}
	}
normalized:
	if generic := strings.IndexByte(text, '<'); generic >= 0 {
		text = text[:generic]
	}
	if generic := strings.IndexByte(text, '['); generic >= 0 && !strings.HasPrefix(text, "[") {
		text = text[:generic]
	}
	text = strings.TrimSpace(strings.TrimPrefix(text, "? extends "))
	text = strings.TrimSpace(strings.TrimPrefix(text, "? super "))
	text = strings.TrimSuffix(strings.TrimSuffix(text, "[]"), "...")
	return text
}

func Multiplicity(text string) string {
	trimmed := strings.TrimSpace(text)
	if strings.HasSuffix(trimmed, "[]") || strings.HasSuffix(trimmed, "...") ||
		strings.HasPrefix(trimmed, "[]") || strings.HasPrefix(trimmed, "map[") ||
		strings.HasPrefix(trimmed, "[") {
		return "*"
	}
	base := Base(trimmed)
	if dot := strings.LastIndex(base, "."); dot >= 0 {
		base = base[dot+1:]
	}
	switch base {
	case "List", "Set", "Collection", "Iterable", "Map", "Queue", "Deque", "Stream":
		return "*"
	case "Optional":
		return "0..1"
	default:
		return "1"
	}
}

func isBuiltin(name string) bool {
	switch name {
	case "byte", "short", "int", "long", "float", "double", "boolean", "char", "void",
		"String", "Integer", "Long", "Short", "Byte", "Float", "Double", "Boolean", "Character",
		"string", "bool", "rune", "uintptr", "uint", "uint8", "uint16", "uint32", "uint64",
		"int8", "int16", "int32", "int64", "float32", "float64", "complex64", "complex128",
		"any", "error", "map", "chan", "func", "interface", "struct":
		return true
	default:
		return false
	}
}

func typeKeyword(name string) bool {
	switch name {
	case "extends", "super", "?":
		return true
	default:
		return isBuiltin(name)
	}
}
