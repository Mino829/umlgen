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

// ParseFile parses Java source with the official Tree-sitter Java grammar and
// converts declaration nodes into umlgen's language-neutral model.
func ParseFile(path string) ([]model.Type, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSource(path, source)
}

// ParseSource parses Java source supplied by the caller. It is also used for
// deleted files loaded from Git history by the diff command.
func ParseSource(path string, source []byte) ([]model.Type, error) {
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(sitter.NewLanguage(tree_sitter_java.Language())); err != nil {
		return nil, fmt.Errorf("initialize Java parser: %w", err)
	}
	tree := parser.Parse(source, nil)
	if tree == nil {
		return nil, fmt.Errorf("Tree-sitter returned no syntax tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	if root.HasError() {
		return nil, syntaxError(root, source)
	}

	pkg := packageName(root, source)
	imports := importDeclarations(root, source)
	var types []model.Type
	for i := uint(0); i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		if !isTypeDeclaration(child.Kind()) {
			continue
		}
		found, err := parseType(child, source, pkg, path, imports, nil)
		if err != nil {
			return nil, err
		}
		types = append(types, found...)
	}
	return types, nil
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
		Imports:    append([]model.Import(nil), imports...),
		Source:     path,
	}
	if superclass := node.ChildByFieldName("superclass"); superclass != nil {
		t.Extends = append(t.Extends, superTypes(superclass, source, "extends")...)
	}
	if interfaces := node.ChildByFieldName("interfaces"); interfaces != nil {
		t.Implements = append(t.Implements, superTypes(interfaces, source, "implements", "extends")...)
	}
	if t.Kind == model.Interface {
		for i := uint(0); i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			if child.Kind() == "extends_interfaces" {
				t.Extends = append(t.Extends, superTypes(child, source, "extends")...)
			}
		}
	}

	if t.Kind == model.Record {
		for _, p := range parseParameters(node.ChildByFieldName("parameters"), source) {
			t.Fields = append(t.Fields, model.Field{Name: p.Name, Type: p.Type, Visibility: model.Private})
		}
	}
	parseBody(&t, bodyNode, source)
	result := []model.Type{t}
	nextEnclosing := append(append([]string(nil), enclosing...), t.Name)
	for i := uint(0); i < bodyNode.NamedChildCount(); i++ {
		child := bodyNode.NamedChild(i)
		if !isTypeDeclaration(child.Kind()) {
			continue
		}
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
		case "method_declaration":
			t.Methods = append(t.Methods, parseMethod(t, child, source, false))
		case "constructor_declaration", "compact_constructor_declaration":
			t.Methods = append(t.Methods, parseMethod(t, child, source, true))
		case "enum_body_declarations":
			parseBody(t, child, source)
		}
	}
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
		Parameters:  parseParameters(node.ChildByFieldName("parameters"), source),
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
		})
	}
	return result
}

func packageName(root *sitter.Node, source []byte) string {
	for i := uint(0); i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		if child.Kind() != "package_declaration" {
			continue
		}
		for j := uint(0); j < child.NamedChildCount(); j++ {
			part := child.NamedChild(j)
			if part.Kind() == "identifier" || part.Kind() == "scoped_identifier" {
				return part.Utf8Text(source)
			}
		}
	}
	return ""
}

func importDeclarations(root *sitter.Node, source []byte) []model.Import {
	var result []model.Import
	for i := uint(0); i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		if child.Kind() != "import_declaration" {
			continue
		}
		text := strings.TrimSpace(child.Utf8Text(source))
		text = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(text, "import"), ";"))
		item := model.Import{}
		if strings.HasPrefix(text, "static ") {
			item.Static = true
			text = strings.TrimSpace(strings.TrimPrefix(text, "static "))
		}
		if strings.HasSuffix(text, ".*") {
			item.Wildcard = true
			text = strings.TrimSuffix(text, ".*")
		}
		item.Name = text
		result = append(result, item)
	}
	return result
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

func superTypes(node *sitter.Node, source []byte, prefixes ...string) []string {
	text := strings.TrimSpace(node.Utf8Text(source))
	for _, prefix := range prefixes {
		if strings.HasPrefix(text, prefix) {
			text = strings.TrimSpace(strings.TrimPrefix(text, prefix))
			break
		}
	}
	var result []string
	for _, item := range splitTopLevel(text, ',') {
		if value := strings.TrimSpace(item); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func splitTopLevel(text string, separator rune) []string {
	var result []string
	start, angle, square, paren := 0, 0, 0, 0
	for i, r := range text {
		switch r {
		case '<':
			angle++
		case '>':
			angle--
		case '[':
			square++
		case ']':
			square--
		case '(':
			paren++
		case ')':
			paren--
		}
		if r == separator && angle == 0 && square == 0 && paren == 0 {
			result = append(result, text[start:i])
			start = i + 1
		}
	}
	return append(result, text[start:])
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
	case "interface_declaration", "annotation_type_declaration":
		return model.Interface
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

func syntaxError(root *sitter.Node, source []byte) error {
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
		return fmt.Errorf("Java syntax error at line %d near %q", bad.StartPosition().Row+1, snippet)
	}
	return fmt.Errorf("Java syntax error")
}

func SortTypes(types []model.Type) {
	sort.Slice(types, func(i, j int) bool {
		return types[i].QualifiedName() < types[j].QualifiedName()
	})
}
