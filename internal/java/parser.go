package java

import (
	"fmt"
	"os"
	"sort"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"

	"github.com/Mino829/umlgen/internal/model"
)

// SchemaVersion identifies the persisted Java source-unit syntax model.
const SchemaVersion = "java-tree-sitter-v3"

// ParseFile parses Java source with the official Tree-sitter Java grammar and
// converts declaration nodes into umlgen's language-neutral model.
func ParseFile(path string) (*model.SourceUnit, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSource(path, source)
}

// ParseSource parses Java source supplied by the caller. It is also used for
// deleted files loaded from Git history by the diff command.
func ParseSource(path string, source []byte) (*model.SourceUnit, error) {
	unit := model.NewSourceUnit("java", path, source)
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(sitter.NewLanguage(tree_sitter_java.Language())); err != nil {
		return unit, fmt.Errorf("initialize Java parser: %w", err)
	}
	tree := parser.Parse(source, nil)
	if tree == nil {
		return unit, fmt.Errorf("Tree-sitter returned no syntax tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	unit.Package = packageDeclaration(root, source)
	unit.Imports = importDeclarations(root, source)
	for i := uint(0); i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		if child.Kind() == "module_declaration" {
			unit.Diagnostics = append(unit.Diagnostics, model.SourceDiagnostic{
				Code: "unsupported-module-declaration", Span: nodeSpan(child),
			})
		}
	}
	if root.HasError() {
		span, err := syntaxError(root, source)
		unit.Diagnostics = append(unit.Diagnostics, model.SourceDiagnostic{Code: "syntax-error", Span: span})
		return unit, err
	}

	var types []model.Type
	for i := uint(0); i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		if !isTypeDeclaration(child.Kind()) {
			continue
		}
		found, err := parseType(child, source, unit.Package.Name, path, unit.Imports, nil)
		if err != nil {
			return unit, err
		}
		types = append(types, found...)
	}
	unit.Types = types
	return unit, nil
}

func parseType(node *sitter.Node, source []byte, pkg, path string, imports []model.Import, enclosing []string) ([]model.Type, error) {
	nameNode := node.ChildByFieldName("name")
	bodyNode := node.ChildByFieldName("body")
	if nameNode == nil || bodyNode == nil {
		return nil, fmt.Errorf("invalid %s declaration at line %d", node.Kind(), node.StartPosition().Row+1)
	}

	t := model.Type{
		Package:    pkg,
		Name:       nameNode.Utf8Text(source),
		Enclosing:  append([]string(nil), enclosing...),
		Kind:       kindFor(node.Kind()),
		Visibility: declarationVisibility(node, source, model.Package),
		Abstract:   hasModifier(node, source, "abstract"),
		Final:      hasModifier(node, source, "final"),
		Sealed:     hasModifier(node, source, "sealed"),
		NonSealed:  hasModifier(node, source, "non-sealed"),
		Imports:    imports,
		Source:     path,
		Line:       int(node.StartPosition().Row) + 1,
	}
	if typeParameters := node.ChildByFieldName("type_parameters"); typeParameters != nil {
		t.TypeParameters = typeText(typeParameters, source)
	}
	if superclass := node.ChildByFieldName("superclass"); superclass != nil {
		t.Extends = append(t.Extends, superTypes(superclass, source, path)...)
	}
	if interfaces := node.ChildByFieldName("interfaces"); interfaces != nil {
		t.Implements = append(t.Implements, superTypes(interfaces, source, path)...)
	}
	if t.Kind == model.Interface {
		for i := uint(0); i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			if child.Kind() == "extends_interfaces" {
				t.Extends = append(t.Extends, superTypes(child, source, path)...)
			}
		}
	}

	if t.Kind == model.Record {
		for _, p := range parseParameters(node.ChildByFieldName("parameters"), source) {
			t.Fields = append(t.Fields, model.Field{Name: p.Name, Type: p.Type, Visibility: model.Private, Source: path, Line: p.Line})
		}
	}
	parseBody(&t, bodyNode, source)
	result := []model.Type{t}
	nextEnclosing := append(append([]string(nil), enclosing...), t.Name)
	for _, child := range nestedTypeDeclarations(bodyNode) {
		nested, err := parseType(child, source, pkg, path, imports, nextEnclosing)
		if err != nil {
			return nil, err
		}
		result = append(result, nested...)
	}
	return result, nil
}

func parseBody(t *model.Type, body *sitter.Node, source []byte) {
	for i := uint(0); i < body.NamedChildCount(); i++ {
		child := body.NamedChild(i)
		switch child.Kind() {
		case "field_declaration", "constant_declaration":
			parseFields(t, child, source)
		case "enum_constant":
			if name := child.ChildByFieldName("name"); name != nil {
				t.EnumValues = append(t.EnumValues, name.Utf8Text(source))
			}
		case "method_declaration":
			t.Methods = append(t.Methods, parseMethod(t, child, source, false))
		case "annotation_type_element_declaration":
			t.Methods = append(t.Methods, parseAnnotationElement(t, child, source))
		case "constructor_declaration", "compact_constructor_declaration":
			t.Methods = append(t.Methods, parseMethod(t, child, source, true))
		case "enum_body_declarations":
			parseBody(t, child, source)
		}
	}
}

func nestedTypeDeclarations(body *sitter.Node) []*sitter.Node {
	var result []*sitter.Node
	for i := uint(0); i < body.NamedChildCount(); i++ {
		child := body.NamedChild(i)
		if isTypeDeclaration(child.Kind()) {
			result = append(result, child)
			continue
		}
		if child.Kind() == "enum_body_declarations" {
			result = append(result, nestedTypeDeclarations(child)...)
		}
	}
	return result
}

func parseAnnotationElement(t *model.Type, node *sitter.Node, source []byte) model.Method {
	method := model.Method{
		Visibility: model.Public,
		Abstract:   true,
		Source:     t.Source,
		Line:       int(node.StartPosition().Row) + 1,
	}
	if name := node.ChildByFieldName("name"); name != nil {
		method.Name = name.Utf8Text(source)
	}
	if returnType := node.ChildByFieldName("type"); returnType != nil {
		method.ReturnType = typeText(returnType, source)
		if dimensions := node.ChildByFieldName("dimensions"); dimensions != nil {
			method.ReturnType += typeText(dimensions, source)
		}
	}
	return method
}

func parseFields(t *model.Type, node *sitter.Node, source []byte) {
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return
	}
	fieldType := typeText(typeNode, source)
	visibility := declarationVisibility(node, source, model.Package)
	isStatic := hasModifier(node, source, "static")
	if t.Kind == model.Interface {
		visibility = model.Public
		isStatic = true
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() != "variable_declarator" {
			continue
		}
		name := child.ChildByFieldName("name")
		if name == nil {
			continue
		}
		declaredType := fieldType
		if dimensions := child.ChildByFieldName("dimensions"); dimensions != nil {
			declaredType += typeText(dimensions, source)
		}
		t.Fields = append(t.Fields, model.Field{
			Name:       name.Utf8Text(source),
			Type:       declaredType,
			Visibility: visibility,
			Static:     isStatic,
			Source:     t.Source,
			Line:       int(node.StartPosition().Row) + 1,
		})
	}
}

func parseMethod(t *model.Type, node *sitter.Node, source []byte, constructor bool) model.Method {
	nameNode := node.ChildByFieldName("name")
	name := t.Name
	if nameNode != nil {
		name = nameNode.Utf8Text(source)
	}
	visibility := declarationVisibility(node, source, model.Package)
	if t.Kind == model.Interface && visibility == model.Package {
		visibility = model.Public
	}
	m := model.Method{
		Name:        name,
		Constructor: constructor,
		Visibility:  visibility,
		Static:      hasModifier(node, source, "static"),
		Abstract:    hasModifier(node, source, "abstract") || (t.Kind == model.Interface && !hasModifier(node, source, "default") && !hasModifier(node, source, "static") && !hasModifier(node, source, "private")),
		Parameters:  parseParameters(node.ChildByFieldName("parameters"), source),
		Source:      t.Source,
		Line:        int(node.StartPosition().Row) + 1,
	}
	if !constructor {
		if returnType := node.ChildByFieldName("type"); returnType != nil {
			m.ReturnType = typeText(returnType, source)
			if dimensions := node.ChildByFieldName("dimensions"); dimensions != nil {
				m.ReturnType += typeText(dimensions, source)
			}
		}
	}
	return m
}

func parseParameters(parameters *sitter.Node, source []byte) []model.Parameter {
	if parameters == nil {
		return nil
	}
	var result []model.Parameter
	for i := uint(0); i < parameters.NamedChildCount(); i++ {
		param := parameters.NamedChild(i)
		switch param.Kind() {
		case "formal_parameter", "spread_parameter":
		default:
			continue
		}
		nameNode := param.ChildByFieldName("name")
		typeNode := param.ChildByFieldName("type")
		if nameNode == nil || typeNode == nil {
			continue
		}
		paramType := typeText(typeNode, source)
		if dimensions := param.ChildByFieldName("dimensions"); dimensions != nil {
			paramType += typeText(dimensions, source)
		}
		if param.Kind() == "spread_parameter" && !strings.HasSuffix(paramType, "...") {
			paramType += "..."
		}
		result = append(result, model.Parameter{
			Name: nameNode.Utf8Text(source),
			Type: paramType,
			Line: int(param.StartPosition().Row) + 1,
		})
	}
	return result
}

func packageDeclaration(root *sitter.Node, source []byte) model.PackageDeclaration {
	for i := uint(0); i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		if child.Kind() != "package_declaration" {
			continue
		}
		for j := uint(0); j < child.NamedChildCount(); j++ {
			part := child.NamedChild(j)
			if part.Kind() == "identifier" || part.Kind() == "scoped_identifier" {
				return model.PackageDeclaration{Name: qualifiedIdentifier(part, source), Span: nodeSpan(child)}
			}
		}
	}
	return model.PackageDeclaration{}
}

func importDeclarations(root *sitter.Node, source []byte) []model.Import {
	var result []model.Import
	for i := uint(0); i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		if child.Kind() != "import_declaration" {
			continue
		}
		item := model.Import{Span: nodeSpan(child)}
		for j := uint(0); j < child.ChildCount(); j++ {
			part := child.Child(j)
			switch part.Kind() {
			case "static":
				item.Static = true
			case "asterisk":
				item.Wildcard = true
			case "identifier", "scoped_identifier":
				item.Name = qualifiedIdentifier(part, source)
			}
		}
		result = append(result, item)
	}
	return result
}

// Qualified names are assembled from identifier nodes so comments and spacing
// around separators never become part of an import or package name.
func qualifiedIdentifier(node *sitter.Node, source []byte) string {
	var name strings.Builder
	appendQualifiedIdentifier(&name, node, source)
	return name.String()
}

func appendQualifiedIdentifier(name *strings.Builder, node *sitter.Node, source []byte) {
	if node.Kind() != "scoped_identifier" {
		name.WriteString(node.Utf8Text(source))
		return
	}
	appendQualifiedIdentifier(name, node.ChildByFieldName("scope"), source)
	name.WriteByte('.')
	appendQualifiedIdentifier(name, node.ChildByFieldName("name"), source)
}

func nodeSpan(node *sitter.Node) model.SourceSpan {
	start, end := node.StartPosition(), node.EndPosition()
	return model.SourceSpan{
		Start: model.SourcePosition{Offset: int(node.StartByte()), Line: int(start.Row) + 1, Column: int(start.Column) + 1},
		End:   model.SourcePosition{Offset: int(node.EndByte()), Line: int(end.Row) + 1, Column: int(end.Column) + 1},
	}
}

func declarationVisibility(node *sitter.Node, source []byte, fallback model.Visibility) model.Visibility {
	modifiers := modifierWords(node, source)
	switch {
	case modifiers["public"]:
		return model.Public
	case modifiers["protected"]:
		return model.Protected
	case modifiers["private"]:
		return model.Private
	default:
		return fallback
	}
}

func hasModifier(node *sitter.Node, source []byte, modifier string) bool {
	return modifierWords(node, source)[modifier]
}

func modifierWords(node *sitter.Node, source []byte) map[string]bool {
	result := map[string]bool{}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() != "modifiers" {
			continue
		}
		for _, word := range strings.Fields(child.Utf8Text(source)) {
			result[word] = true
		}
		break
	}
	return result
}

func superTypes(node *sitter.Node, source []byte, path string) []model.TypeReference {
	var result []model.TypeReference
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "type_list":
			result = append(result, superTypes(child, source, path)...)
		case "line_comment", "block_comment":
			continue
		default:
			result = append(result, model.TypeReference{
				Name:   strings.TrimSpace(child.Utf8Text(source)),
				Source: path,
				Line:   int(child.StartPosition().Row) + 1,
			})
		}
	}
	return result
}

func typeText(node *sitter.Node, source []byte) string {
	text := strings.Join(strings.Fields(node.Utf8Text(source)), " ")
	replacer := strings.NewReplacer(
		" <", "<", "< ", "<", " >", ">", "> ", ">",
		" ,", ",", ", ", ", ",
		" [", "[", "[ ", "[", " ]", "]", "] ", "]",
		" . ", ".", " .", ".", ". ", ".",
	)
	return replacer.Replace(text)
}

func kindFor(nodeKind string) model.TypeKind {
	switch nodeKind {
	case "interface_declaration":
		return model.Interface
	case "annotation_type_declaration":
		return model.Annotation
	case "enum_declaration":
		return model.Enum
	case "record_declaration":
		return model.Record
	default:
		return model.Class
	}
}

func isTypeDeclaration(kind string) bool {
	switch kind {
	case "class_declaration", "interface_declaration", "annotation_type_declaration",
		"enum_declaration", "record_declaration":
		return true
	default:
		return false
	}
}

func syntaxError(root *sitter.Node, source []byte) (model.SourceSpan, error) {
	var find func(*sitter.Node) *sitter.Node
	find = func(node *sitter.Node) *sitter.Node {
		if node.IsError() || node.IsMissing() {
			return node
		}
		for i := uint(0); i < node.ChildCount(); i++ {
			if bad := find(node.Child(i)); bad != nil {
				return bad
			}
		}
		return nil
	}
	if bad := find(root); bad != nil {
		snippet := strings.TrimSpace(bad.Utf8Text(source))
		if len(snippet) > 80 {
			snippet = snippet[:80] + "..."
		}
		return nodeSpan(bad), fmt.Errorf("Java syntax error at line %d near %q", bad.StartPosition().Row+1, snippet)
	}
	return nodeSpan(root), fmt.Errorf("Java syntax error")
}

func SortTypes(types []model.Type) {
	sort.Slice(types, func(i, j int) bool {
		return types[i].QualifiedName() < types[j].QualifiedName()
	})
}
