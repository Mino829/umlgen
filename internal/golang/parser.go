package golang

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/relations"
)

const SchemaVersion = "go-ast-v1"

func ParseFile(path string) ([]model.Type, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSource(path, source)
}

func ParseSource(path string, source []byte) ([]model.Type, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, source, parser.AllErrors)
	if err != nil {
		return nil, fmt.Errorf("Go syntax error: %w", err)
	}

	pkg := packagePath(path, file.Name.Name)
	imports := parseImports(file)
	var types []model.Type
	byName := map[string]int{}
	for _, declaration := range file.Decls {
		generic, ok := declaration.(*ast.GenDecl)
		if !ok || generic.Tok != token.TYPE {
			continue
		}
		for _, spec := range generic.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			parsed := parseType(typeSpec, fset, pkg, path, imports)
			byName[parsed.Name] = len(types)
			types = append(types, parsed)
		}
	}

	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv == nil || len(function.Recv.List) == 0 {
			continue
		}
		receiver := receiverName(function.Recv.List[0].Type)
		if receiver == "" {
			continue
		}
		method := parseMethod(function.Name.Name, function.Type, fset)
		method.Visibility = visibility(function.Name.Name)
		if index, ok := byName[receiver]; ok {
			types[index].Methods = append(types[index].Methods, method)
			continue
		}
		byName[receiver] = len(types)
		types = append(types, model.Type{
			Package: pkg, Name: receiver, Kind: model.Struct,
			Visibility: visibility(receiver), Imports: cloneImports(imports),
			Methods: []model.Method{method}, Source: path,
		})
	}
	SortTypes(types)
	return types, nil
}

func parseType(spec *ast.TypeSpec, fset *token.FileSet, pkg, path string, imports []model.Import) model.Type {
	t := model.Type{
		Package: pkg, Name: spec.Name.Name, Kind: model.Class,
		Visibility: visibility(spec.Name.Name), Imports: cloneImports(imports), Source: path,
	}
	switch declared := spec.Type.(type) {
	case *ast.StructType:
		t.Kind = model.Struct
		parseStruct(&t, declared, fset)
	case *ast.InterfaceType:
		t.Kind = model.Interface
		parseInterface(&t, declared, fset)
	default:
		t.Fields = append(t.Fields, model.Field{
			Name: "value", Type: expressionText(fset, spec.Type), Visibility: model.Private,
		})
	}
	return t
}

func parseStruct(t *model.Type, declared *ast.StructType, fset *token.FileSet) {
	for _, field := range declared.Fields.List {
		fieldType := expressionText(fset, field.Type)
		if len(field.Names) == 0 {
			t.Extends = append(t.Extends, fieldType)
			continue
		}
		for _, name := range field.Names {
			t.Fields = append(t.Fields, model.Field{
				Name: name.Name, Type: fieldType, Visibility: visibility(name.Name),
			})
		}
	}
}

func parseInterface(t *model.Type, declared *ast.InterfaceType, fset *token.FileSet) {
	for _, field := range declared.Methods.List {
		if len(field.Names) == 0 {
			t.Extends = append(t.Extends, expressionText(fset, field.Type))
			continue
		}
		function, ok := field.Type.(*ast.FuncType)
		if !ok {
			continue
		}
		for _, name := range field.Names {
			method := parseMethod(name.Name, function, fset)
			method.Visibility = visibility(name.Name)
			t.Methods = append(t.Methods, method)
		}
	}
}

func parseMethod(name string, function *ast.FuncType, fset *token.FileSet) model.Method {
	return model.Method{
		Name: name, Visibility: visibility(name),
		Parameters: parseParameters(function.Params, fset),
		ReturnType: parseResults(function.Results, fset),
	}
}

func parseParameters(fields *ast.FieldList, fset *token.FileSet) []model.Parameter {
	if fields == nil {
		return nil
	}
	var result []model.Parameter
	unnamed := 0
	for _, field := range fields.List {
		parameterType := expressionText(fset, field.Type)
		if len(field.Names) == 0 {
			unnamed++
			result = append(result, model.Parameter{Name: fmt.Sprintf("arg%d", unnamed), Type: parameterType})
			continue
		}
		for _, name := range field.Names {
			result = append(result, model.Parameter{Name: name.Name, Type: parameterType})
		}
	}
	return result
}

func parseResults(fields *ast.FieldList, fset *token.FileSet) string {
	if fields == nil || len(fields.List) == 0 {
		return ""
	}
	var result []string
	for _, field := range fields.List {
		resultType := expressionText(fset, field.Type)
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for range count {
			result = append(result, resultType)
		}
	}
	if len(result) == 1 {
		return result[0]
	}
	return "(" + strings.Join(result, ", ") + ")"
}

func parseImports(file *ast.File) []model.Import {
	var result []model.Import
	for _, imported := range file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			continue
		}
		item := model.Import{Name: path}
		if imported.Name != nil {
			item.Alias = imported.Name.Name
		}
		result = append(result, item)
	}
	return result
}

func expressionText(fset *token.FileSet, expression ast.Expr) string {
	var output bytes.Buffer
	if err := format.Node(&output, fset, expression); err != nil {
		return ""
	}
	return output.String()
}

func receiverName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return receiverName(value.X)
	case *ast.IndexExpr:
		return receiverName(value.X)
	case *ast.IndexListExpr:
		return receiverName(value.X)
	default:
		return ""
	}
}

func visibility(name string) model.Visibility {
	for _, first := range name {
		if unicode.IsUpper(first) {
			return model.Public
		}
		break
	}
	return model.Private
}

func packagePath(sourcePath, packageName string) string {
	directory := filepath.Dir(sourcePath)
	absolute, err := filepath.Abs(directory)
	if err == nil {
		directory = absolute
	}
	for current := directory; ; current = filepath.Dir(current) {
		moduleFile := filepath.Join(current, "go.mod")
		if data, readErr := os.ReadFile(moduleFile); readErr == nil {
			module := moduleName(string(data))
			if module == "" {
				break
			}
			relative, relErr := filepath.Rel(current, directory)
			if relErr != nil || relative == "." {
				return module
			}
			return strings.TrimSuffix(module, "/") + "/" + filepath.ToSlash(relative)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return packageName
}

func moduleName(content string) string {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "module" {
			return fields[1]
		}
	}
	return ""
}

func Finalize(types []model.Type) []model.Type {
	merged := make([]model.Type, 0, len(types))
	indexes := map[string]int{}
	for _, current := range types {
		key := current.QualifiedName()
		if index, ok := indexes[key]; ok {
			mergeType(&merged[index], current)
			continue
		}
		indexes[key] = len(merged)
		merged = append(merged, current)
	}
	inferImplementations(merged)
	SortTypes(merged)
	return merged
}

func mergeType(target *model.Type, source model.Type) {
	if target.Kind == model.Struct && source.Kind != model.Struct {
		target.Kind = source.Kind
	}
	target.Fields = appendUniqueFields(target.Fields, source.Fields...)
	target.Methods = appendUniqueMethods(target.Methods, source.Methods...)
	target.Extends = appendUnique(target.Extends, source.Extends...)
	target.Implements = appendUnique(target.Implements, source.Implements...)
	if source.Change != model.Unchanged {
		target.Change = source.Change
	}
	for _, imported := range source.Imports {
		found := false
		for _, existing := range target.Imports {
			if existing == imported {
				found = true
				break
			}
		}
		if !found {
			target.Imports = append(target.Imports, imported)
		}
	}
}

func inferImplementations(types []model.Type) {
	index := relations.NewIndex(types)
	for typeIndex := range types {
		candidate := &types[typeIndex]
		if candidate.Kind == model.Interface {
			continue
		}
		for _, contract := range types {
			if contract.Kind != model.Interface {
				continue
			}
			required := methodSet(contract, index, map[string]bool{})
			if len(required) == 0 {
				continue
			}
			available := methodSet(*candidate, index, map[string]bool{})
			if implements(available, required) {
				candidate.Implements = appendUnique(candidate.Implements, contract.QualifiedName())
			}
		}
	}
}

func methodSet(current model.Type, index *relations.Index, seen map[string]bool) []model.Method {
	qualified := current.QualifiedName()
	if seen[qualified] {
		return nil
	}
	seen[qualified] = true
	methods := append([]model.Method(nil), current.Methods...)
	for _, embedded := range current.Extends {
		resolved, ok := index.Resolve(current, embedded)
		if !ok {
			continue
		}
		parent, ok := index.Type(resolved)
		if !ok {
			continue
		}
		methods = appendUniqueMethods(methods, methodSet(parent, index, seen)...)
	}
	return methods
}

func implements(methods, required []model.Method) bool {
	available := map[string]bool{}
	for _, method := range methods {
		available[methodSignature(method)] = true
	}
	for _, method := range required {
		if !available[methodSignature(method)] {
			return false
		}
	}
	return true
}

func methodSignature(method model.Method) string {
	var parameters []string
	for _, parameter := range method.Parameters {
		parameters = append(parameters, parameter.Type)
	}
	return method.Name + "(" + strings.Join(parameters, ",") + ")" + method.ReturnType
}

func appendUnique(values []string, additional ...string) []string {
	seen := make(map[string]bool, len(values)+len(additional))
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additional {
		if value != "" && !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	return values
}

func appendUniqueFields(fields []model.Field, additional ...model.Field) []model.Field {
	seen := map[string]bool{}
	for _, field := range fields {
		seen[field.Name+"\x00"+field.Type] = true
	}
	for _, field := range additional {
		key := field.Name + "\x00" + field.Type
		if !seen[key] {
			fields = append(fields, field)
			seen[key] = true
		}
	}
	return fields
}

func appendUniqueMethods(methods []model.Method, additional ...model.Method) []model.Method {
	seen := map[string]bool{}
	for _, method := range methods {
		seen[methodSignature(method)] = true
	}
	for _, method := range additional {
		key := methodSignature(method)
		if !seen[key] {
			methods = append(methods, method)
			seen[key] = true
		}
	}
	return methods
}

func cloneImports(imports []model.Import) []model.Import {
	return append([]model.Import(nil), imports...)
}

func SortTypes(types []model.Type) {
	sort.Slice(types, func(i, j int) bool {
		return types[i].QualifiedName() < types[j].QualifiedName()
	})
}
