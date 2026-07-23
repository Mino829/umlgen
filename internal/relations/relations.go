package relations

import (
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
}

type Index struct {
	types     []model.Type
	qualified map[string]int
	simple    map[string][]int
}

func NewIndex(types []model.Type) *Index {
	index := &Index{
		types:     types,
		qualified: make(map[string]int, len(types)),
		simple:    make(map[string][]int, len(types)),
	}
	for i, t := range types {
		index.qualified[t.QualifiedName()] = i
		index.simple[t.Name] = append(index.simple[t.Name], i)
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

	simpleName := reference
	if dot := strings.LastIndex(reference, "."); dot >= 0 {
		simpleName = reference[dot+1:]
	}
	for _, imported := range owner.Imports {
		if imported.Static || imported.Wildcard {
			continue
		}
		if imported.Name == reference || strings.HasSuffix(imported.Name, "."+simpleName) {
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
	if matches := i.simple[simpleName]; len(matches) == 1 {
		return i.types[matches[0]].QualifiedName(), true
	}
	return "", false
}

func (i *Index) has(name string) bool {
	_, ok := i.qualified[name]
	return ok
}

func Build(types []model.Type) []Relation {
	index := NewIndex(types)
	unique := map[string]Relation{}
	add := func(owner model.Type, raw string, kind Kind, label, multiplicity string) {
		for _, ref := range References(raw) {
			target, ok := index.Resolve(owner, ref)
			if !ok || target == owner.QualifiedName() {
				continue
			}
			relation := Relation{
				From: owner.QualifiedName(), To: target, Kind: kind,
				Label: label, Multiplicity: multiplicity,
			}
			key := strings.Join([]string{relation.From, relation.To, string(kind), label, multiplicity}, "\x00")
			unique[key] = relation
		}
	}
	for _, t := range types {
		for _, parent := range t.Extends {
			if target, ok := index.Resolve(t, parent); ok && target != t.QualifiedName() {
				r := Relation{From: t.QualifiedName(), To: target, Kind: Inheritance}
				unique[relationKey(r)] = r
			}
		}
		for _, parent := range t.Implements {
			if target, ok := index.Resolve(t, parent); ok && target != t.QualifiedName() {
				r := Relation{From: t.QualifiedName(), To: target, Kind: Implementation}
				unique[relationKey(r)] = r
			}
		}
		for _, field := range t.Fields {
			add(t, field.Type, Field, field.Name, Multiplicity(field.Type))
		}
		for _, method := range t.Methods {
			for _, parameter := range method.Parameters {
				add(t, parameter.Type, Parameter, method.Name+"("+parameter.Name+")", Multiplicity(parameter.Type))
			}
			if !method.Constructor {
				add(t, method.ReturnType, Return, method.Name, Multiplicity(method.ReturnType))
			}
		}
	}
	result := make([]Relation, 0, len(unique))
	for _, relation := range unique {
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
	if generic := strings.IndexByte(text, '<'); generic >= 0 {
		text = text[:generic]
	}
	text = strings.TrimSpace(strings.TrimPrefix(text, "? extends "))
	text = strings.TrimSpace(strings.TrimPrefix(text, "? super "))
	text = strings.TrimSuffix(strings.TrimSuffix(text, "[]"), "...")
	return text
}

func Multiplicity(text string) string {
	trimmed := strings.TrimSpace(text)
	if strings.HasSuffix(trimmed, "[]") || strings.HasSuffix(trimmed, "...") {
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
		"String", "Integer", "Long", "Short", "Byte", "Float", "Double", "Boolean", "Character":
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
