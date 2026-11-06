package golang

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/scanner"
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

const SchemaVersion = "go-ast-v4"

func ParseFile(path string) (*model.SourceUnit, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSource(path, source)
}

// ParseSource returns a content-dependent source unit. BindContext must run
// before package filtering, finalization or relation analysis.
func ParseSource(path string, source []byte) (*model.SourceUnit, error) {
	unit := model.NewSourceUnit("go", path, source)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, source, parser.AllErrors)
	if err != nil {
		if syntaxErrors, ok := err.(scanner.ErrorList); ok {
			var sourceFile *token.File
			fset.Iterate(func(file *token.File) bool {
				sourceFile = file
				return false
			})
			for _, syntaxError := range syntaxErrors {
				physical := sourceFile.PositionFor(sourceFile.Pos(syntaxError.Pos.Offset), false)
				position := model.SourcePosition{Offset: physical.Offset, Line: physical.Line, Column: physical.Column}
				unit.Diagnostics = append(unit.Diagnostics, model.SourceDiagnostic{
					Code: "syntax-error", Span: model.SourceSpan{Start: position, End: position},
				})
			}
		}
		return unit, fmt.Errorf("Go syntax error: %w", err)
	}

	pkg := file.Name.Name
	unit.Package = model.PackageDeclaration{
		Name: pkg, Span: sourceSpan(fset, file.Package, file.Name.End()),
	}
	unit.Imports = parseImports(file, fset)
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
			parsed := parseType(typeSpec, fset, pkg, path, unit.Imports)
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
		method := parseMethod(function.Name.Name, function.Type, fset, path, function.Pos())
		method.Visibility = visibility(function.Name.Name)
		method.PointerReceiver = isPointerReceiver(function.Recv.List[0].Type)
		if index, ok := byName[receiver]; ok {
			types[index].Methods = append(types[index].Methods, method)
			continue
		}
		byName[receiver] = len(types)
		types = append(types, model.Type{
			Package: pkg, Name: receiver, Kind: model.Struct,
			Visibility: visibility(receiver), Imports: unit.Imports,
			Methods: []model.Method{method}, Source: path,
		})
	}
	SortTypes(types)
	unit.Types = types
	return unit, nil
}

func parseType(spec *ast.TypeSpec, fset *token.FileSet, pkg, path string, imports []model.Import) model.Type {
	t := model.Type{
		Package: pkg, Name: spec.Name.Name, Kind: model.Class,
		Visibility: visibility(spec.Name.Name), Imports: imports, Source: path,
		Line: fset.PositionFor(spec.Pos(), false).Line,
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
			Source: path, Line: fset.PositionFor(spec.Pos(), false).Line,
		})
	}
	return t
}

func parseStruct(t *model.Type, declared *ast.StructType, fset *token.FileSet) {
	for _, field := range declared.Fields.List {
		fieldType := expressionText(fset, field.Type)
		if len(field.Names) == 0 {
			t.Extends = append(t.Extends, model.TypeReference{
				Name: fieldType, Source: t.Source, Line: fset.PositionFor(field.Type.Pos(), false).Line,
			})
			continue
		}
		for _, name := range field.Names {
			t.Fields = append(t.Fields, model.Field{
				Name: name.Name, Type: fieldType, Visibility: visibility(name.Name),
				Source: t.Source, Line: fset.PositionFor(name.Pos(), false).Line,
			})
		}
	}
}

func parseInterface(t *model.Type, declared *ast.InterfaceType, fset *token.FileSet) {
	for _, field := range declared.Methods.List {
		if len(field.Names) == 0 {
			t.Extends = append(t.Extends, model.TypeReference{
				Name: expressionText(fset, field.Type), Source: t.Source,
				Line: fset.PositionFor(field.Type.Pos(), false).Line,
			})
			continue
		}
		function, ok := field.Type.(*ast.FuncType)
		if !ok {
			continue
		}
		for _, name := range field.Names {
			method := parseMethod(name.Name, function, fset, t.Source, name.Pos())
			method.Visibility = visibility(name.Name)
			t.Methods = append(t.Methods, method)
		}
	}
}

func parseMethod(name string, function *ast.FuncType, fset *token.FileSet, source string, position token.Pos) model.Method {
	return model.Method{
		Name: name, Visibility: visibility(name),
		Source: source, Line: fset.PositionFor(position, false).Line,
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
			result = append(result, model.Parameter{
				Name: fmt.Sprintf("arg%d", unnamed), Type: parameterType,
				Line: fset.PositionFor(field.Type.Pos(), false).Line,
			})
			continue
		}
		for _, name := range field.Names {
			result = append(result, model.Parameter{
				Name: name.Name, Type: parameterType, Line: fset.PositionFor(name.Pos(), false).Line,
			})
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

func parseImports(file *ast.File, fset *token.FileSet) []model.Import {
	var result []model.Import
	for _, imported := range file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			continue
		}
		item := model.Import{Name: path, Span: sourceSpan(fset, imported.Pos(), imported.End())}
		if imported.Name != nil {
			item.Alias = imported.Name.Name
		}
		result = append(result, item)
	}
	return result
}

func sourceSpan(fset *token.FileSet, start, end token.Pos) model.SourceSpan {
	startPosition := fset.PositionFor(start, false)
	endPosition := fset.PositionFor(end, false)
	return model.SourceSpan{
		Start: model.SourcePosition{Offset: startPosition.Offset, Line: startPosition.Line, Column: startPosition.Column},
		End:   model.SourcePosition{Offset: endPosition.Offset, Line: endPosition.Line, Column: endPosition.Column},
	}
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

func isPointerReceiver(expression ast.Expr) bool {
	switch value := expression.(type) {
	case *ast.StarExpr:
		return true
	case *ast.IndexExpr:
		return isPointerReceiver(value.X)
	case *ast.IndexListExpr:
		return isPointerReceiver(value.X)
	default:
		return false
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

// BindContext binds one file's syntax declarations to its current module and
// directory. It must run after cache storage so module paths are never persisted.
func BindContext(unit *model.SourceUnit) {
	unit.PackagePath, unit.PackageContext = packagePath(unit.Path, unit.Package.Name)
	for i := range unit.Types {
		unit.Types[i].Package = unit.PackagePath
	}
}

func packagePath(sourcePath, packageName string) (string, string) {
	directory := filepath.Dir(sourcePath)
	absolute, err := filepath.Abs(directory)
	if err == nil {
		directory = absolute
	}
	for current := directory; ; current = filepath.Dir(current) {
		moduleFile := filepath.Join(current, "go.mod")
		if data, readErr := os.ReadFile(moduleFile); readErr == nil {
			module := ModuleName(data)
			if module == "" {
				break
			}
			relative, relErr := filepath.Rel(current, directory)
			if relErr != nil || relative == "." {
				return module, current
			}
			return strings.TrimSuffix(module, "/") + "/" + filepath.ToSlash(relative), current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return packageName, directory
}

// ModuleName reads the parser's lexical module directive without interpreting
// build configuration. Both working-tree and Git-snapshot binding use it.
func ModuleName(content []byte) string {
	for len(content) > 0 {
		line, rest, _ := bytes.Cut(content, []byte{'\n'})
		content = rest
		fields := bytes.Fields(line)
		if len(fields) >= 2 && bytes.Equal(fields[0], []byte("module")) {
			return string(fields[1])
		}
	}
	return ""
}

func Finalize(types []model.Type) []model.Type {
	return finalize(types, false)
}

// FinalizeWithEvidence also records the declarations supporting syntactic
// interface matches; it does not perform compiler-level verification.
func FinalizeWithEvidence(types []model.Type) []model.Type {
	return finalize(types, true)
}

func finalize(types []model.Type, withEvidence bool) []model.Type {
	merged := make([]model.Type, 0, len(types))
	indexes := map[string]int{}
	for _, current := range types {
		current.ImplementationMethods = nil
		key := current.QualifiedName()
		if index, ok := indexes[key]; ok {
			mergeType(&merged[index], current)
			continue
		}
		indexes[key] = len(merged)
		// A class projection may merge imports, but must never append into its
		// source unit's canonical slice. Defer allocation until imports grow.
		current.Imports = current.Imports[:len(current.Imports):len(current.Imports)]
		merged = append(merged, current)
	}
	inferImplementations(merged, withEvidence)
	SortTypes(merged)
	return merged
}

func mergeType(target *model.Type, source model.Type) {
	if target.Line == 0 && source.Line != 0 {
		target.Source = source.Source
		target.Line = source.Line
	}
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
			if existing.Name == imported.Name && existing.Alias == imported.Alias &&
				existing.Static == imported.Static && existing.Wildcard == imported.Wildcard {
				found = true
				break
			}
		}
		if !found {
			target.Imports = append(target.Imports, imported)
		}
	}
}

func inferImplementations(types []model.Type, withEvidence bool) {
	index := relations.NewIndex(types)
	for typeIndex := range types {
		candidate := &types[typeIndex]
		if candidate.Kind == model.Interface {
			continue
		}
		source, line := candidate.Source, candidate.Line
		if line == 0 {
			for _, method := range candidate.Methods {
				if method.Line != 0 {
					source, line = method.Source, method.Line
					break
				}
			}
		}
		for _, contract := range types {
			if contract.Kind != model.Interface {
				continue
			}
			required := methodSetForType(contract, index, map[string]bool{}, false)
			if len(required) == 0 {
				continue
			}
			// Check if either the value type or pointer type implements the interface
			// In UML, a struct represents both T and *T
			valueMethods := methodSetForType(*candidate, index, map[string]bool{}, false)
			pointerMethods := methodSetForType(*candidate, index, map[string]bool{}, true)
			matchedMethods := valueMethods
			if !implements(valueMethods, required) {
				if !implements(pointerMethods, required) {
					continue
				}
				matchedMethods = pointerMethods
			}
			candidate.Implements = appendUnique(candidate.Implements, model.TypeReference{
				Name: contract.QualifiedName(), Source: source, Line: line, Inferred: true,
			})
			if withEvidence {
				candidate.ImplementationMethods = appendImplementationMethods(candidate.ImplementationMethods,
					contract.QualifiedName(), matchedMethods, required)
			}
		}
	}
}

func methodSetForType(current model.Type, index *relations.Index, seen map[string]bool, includePointer bool) []model.Method {
	qualified := current.QualifiedName()
	if seen[qualified] {
		return nil
	}
	seen[qualified] = true
	// Filter methods based on whether we're checking value or pointer type
	methods := make([]model.Method, 0, len(current.Methods))
	for _, method := range current.Methods {
		if includePointer || !method.PointerReceiver {
			methods = append(methods, method)
		}
	}
	for _, embedded := range current.Extends {
		resolved, ok := index.Resolve(current, embedded.Name)
		if !ok {
			continue
		}
		parent, ok := index.Type(resolved)
		if !ok {
			continue
		}
		methods = appendUniqueMethods(methods, methodSetForType(parent, index, seen, includePointer)...)
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

func appendImplementationMethods(evidence []model.ImplementationMethod, contract string, methods, required []model.Method) []model.ImplementationMethod {
	available := make(map[string]int, len(methods))
	for index, method := range methods {
		available[methodSignature(method)] = index
	}
	for _, requirement := range required {
		signature := methodSignature(requirement)
		method := &methods[available[signature]]
		evidence = append(evidence, model.ImplementationMethod{
			Interface: contract, Signature: signature, Source: method.Source, Line: method.Line,
		})
	}
	return evidence
}

func methodSignature(method model.Method) string {
	var parameters []string
	for _, parameter := range method.Parameters {
		parameters = append(parameters, parameter.Type)
	}
	return method.Name + "(" + strings.Join(parameters, ",") + ")" + method.ReturnType
}

func appendUnique(values []model.TypeReference, additional ...model.TypeReference) []model.TypeReference {
	seen := make(map[model.TypeReference]bool, len(values)+len(additional))
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additional {
		if value.Name != "" && !seen[value] {
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

func SortTypes(types []model.Type) {
	sort.Slice(types, func(i, j int) bool {
		return types[i].QualifiedName() < types[j].QualifiedName()
	})
}
