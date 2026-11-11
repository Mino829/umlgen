package relations

import (
	"path/filepath"
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
	Member    string
	Type      string
	Source    string
	Line      int
	Inferred  bool
	Language  string
	OwnerKind model.TypeKind
}

type ReferenceStatus string

const (
	StatusUnresolved     ReferenceStatus = "unresolved"
	StatusOutsideDiagram ReferenceStatus = "outside-diagram"
)

type ReferenceDiagnostic struct {
	From      string
	Kind      Kind
	Reference string
	Target    string
	Evidence  Evidence
	Status    ReferenceStatus
}

// Input separates the selected class declarations from the complete parsed
// context. Known is used only to classify references outside the selection;
// it must not introduce edges or change the existing class resolution policy.
type Input struct {
	Types []model.Type
	Known []model.Type
}

// Analysis is the class projection shared by focus, PlantUML and explanations.
// Project and Relations describe the visible graph; diagnostics and captured
// references are populated only when details are requested.
type Analysis struct {
	Project     model.Project
	Relations   []Relation
	Diagnostics []ReferenceDiagnostic
	references  []ReferenceDiagnostic
}

type Index struct {
	types     []model.Type
	qualified map[string]int
	aliases   map[string]bool
}

func NewIndex(types []model.Type) *Index {
	index := &Index{
		types:     types,
		qualified: make(map[string]int, len(types)),
	}
	for _, t := range types {
		if t.Kind == model.Alias {
			if index.aliases == nil {
				index.aliases = make(map[string]bool)
			}
			index.aliases[t.QualifiedName()] = true
		}
	}
	for i, t := range types {
		name := t.QualifiedName()
		if index.aliases[name] {
			continue
		}
		index.qualified[name] = i
	}
	return index
}

func (i *Index) Resolve(owner model.Type, reference string) (string, bool) {
	reference = Base(reference)
	if reference == "" || isBuiltin(reference) || isTypeParameter(owner, reference) {
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

func isTypeParameter(owner model.Type, reference string) bool {
	for _, name := range owner.TypeParameterNames {
		if reference == name {
			return true
		}
	}
	return false
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

// Analyze builds the selected graph once. Details adds declaration evidence and
// reference diagnostics without changing its edges. Input slices are read-only.
func Analyze(input Input, details bool) Analysis {
	return build(input.Types, input.Known, details, nil, details)
}

// AnalyzeWithResolver reuses declaration relation semantics with a scoped name
// resolver. The boolean identifies inferred parent references, whose canonical
// names must not capture ordinary file-local selectors. Ambiguity is rejected.
func AnalyzeWithResolver(types []model.Type, resolve func(model.Type, string, bool) (string, bool)) Analysis {
	return build(types, nil, true, resolve, false)
}

func build(types, known []model.Type, explain bool, resolve func(model.Type, string, bool) (string, bool), captureReferences bool) Analysis {
	var index *Index
	if resolve == nil {
		index = NewIndex(types)
		resolve = func(owner model.Type, reference string, _ bool) (string, bool) {
			return index.Resolve(owner, reference)
		}
	}
	knownResolve := resolve
	if explain && known != nil {
		index := NewIndex(known)
		knownResolve = func(owner model.Type, reference string, _ bool) (string, bool) {
			return index.Resolve(owner, reference)
		}
	}
	var diagnostics []ReferenceDiagnostic
	var references []ReferenceDiagnostic
	capture := func(owner model.Type, ref, target string, kind Kind, evidence Evidence) {
		if captureReferences {
			references = append(references, ReferenceDiagnostic{
				From: owner.QualifiedName(), Kind: kind, Reference: ref,
				Target: target, Evidence: evidence, Status: StatusOutsideDiagram,
			})
		}
	}
	var seenDiagnostics map[ReferenceDiagnostic]bool
	diagnose := func(owner model.Type, ref string, kind Kind, evidence Evidence) {
		if !explain || ref == "" || isBuiltin(Base(ref)) || isTypeParameter(owner, Base(ref)) {
			return
		}
		diagnostic := ReferenceDiagnostic{
			From: owner.QualifiedName(), Kind: kind, Reference: ref,
			Evidence: evidence, Status: StatusUnresolved,
		}
		if target, ok := knownResolve(owner, ref, evidence.Inferred); ok {
			diagnostic.Target = target
			diagnostic.Status = StatusOutsideDiagram
		}
		if seenDiagnostics == nil {
			seenDiagnostics = make(map[ReferenceDiagnostic]bool)
		}
		if !seenDiagnostics[diagnostic] {
			diagnostics = append(diagnostics, diagnostic)
			seenDiagnostics[diagnostic] = true
		}
	}
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
		if explain {
			evidence = locateEvidence(owner, evidence)
		}
		for _, ref := range References(raw) {
			if isTypeParameter(owner, ref) {
				continue
			}
			target, ok := resolve(owner, ref, evidence.Inferred)
			if !ok {
				diagnose(owner, ref, kind, evidence)
				continue
			}
			if target == owner.QualifiedName() {
				continue
			}
			capture(owner, ref, target, kind, evidence)
			store(Relation{
				From: owner.QualifiedName(), To: target, Kind: kind,
				Label: label, Multiplicity: multiplicity,
			}, evidence)
		}
	}
	for _, t := range types {
		if t.Kind == model.Alias || index != nil && index.aliases != nil && index.aliases[t.QualifiedName()] {
			continue
		}
		for _, parents := range []struct {
			refs []model.TypeReference
			kind Kind
		}{{t.Extends, Inheritance}, {t.Implements, Implementation}} {
			for _, parent := range parents.refs {
				if isTypeParameter(t, Base(parent.Name)) {
					continue
				}
				evidence := Evidence{Type: parent.Name, Source: parent.Source, Line: parent.Line, Inferred: parent.Inferred}
				if explain {
					evidence = locateEvidence(t, evidence)
				}
				if target, ok := resolve(t, parent.Name, parent.Inferred); ok && target != t.QualifiedName() {
					capture(t, Base(parent.Name), target, parents.kind, evidence)
					relation := Relation{From: t.QualifiedName(), To: target, Kind: parents.kind}
					store(relation, evidence)
					if explain && parent.Inferred {
						for _, method := range t.ImplementationMethods {
							if method.Interface == Base(parent.Name) {
								store(relation, locateEvidence(t, Evidence{
									Member: method.Signature, Type: parent.Name,
									Source: method.Source, Line: method.Line, Inferred: true,
								}))
							}
						}
					}
				} else if !ok {
					diagnose(t, Base(parent.Name), parents.kind, evidence)
				}
			}
		}
		for _, field := range t.Fields {
			add(t, field.Type, Field, field.Name, Multiplicity(field.Type), Evidence{
				Member: field.Name, Type: field.Type, Source: field.Source, Line: field.Line,
			})
		}
		for _, method := range t.Methods {
			methodOwner := t
			methodOwner.TypeParameterNames = method.TypeParameterNames
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
				add(methodOwner, parameter.Type, Parameter, method.Name+"("+parameter.Name+")", Multiplicity(parameter.Type), Evidence{
					Member: parameterMember, Type: parameter.Type, Source: method.Source, Line: parameter.Line,
				})
			}
			if !method.Constructor {
				add(methodOwner, method.ReturnType, Return, method.Name, Multiplicity(method.ReturnType), Evidence{
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
	sortDiagnostics(diagnostics)
	return Analysis{Project: model.Project{Types: types}, Relations: result, Diagnostics: diagnostics, references: references}
}

func relationKey(r Relation) string {
	return strings.Join([]string{r.From, r.To, string(r.Kind), r.Label, r.Multiplicity}, "\x00")
}

func locateEvidence(owner model.Type, evidence Evidence) Evidence {
	evidence.OwnerKind = owner.Kind
	switch {
	case strings.EqualFold(filepath.Ext(owner.Source), ".go"):
		evidence.Language = "go"
	case strings.EqualFold(filepath.Ext(owner.Source), ".java"):
		evidence.Language = "java"
	}
	return evidence
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
