package golang

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/scanner"
)

func TestParseAndFinalizeCompatibilityFixture(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "go", "compatibility")
	files, err := scanner.SourceFiles([]string{root}, nil, ".go")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("files = %d, want 3", len(files))
	}

	var parsed []model.Type
	for _, file := range files {
		unit, parseErr := ParseFile(file)
		if parseErr != nil {
			t.Fatalf("ParseFile(%s): %v", file, parseErr)
		}
		BindContext(unit)
		parsed = append(parsed, unit.Types...)
	}
	types := Finalize(parsed)
	byName := map[string]model.Type{}
	for _, parsedType := range types {
		byName[parsedType.QualifiedName()] = parsedType
	}

	user := byName["example.com/shop/domain.User"]
	if user.Kind != model.Struct || len(user.Fields) != 4 || len(user.Methods) != 1 {
		t.Fatalf("user = %#v", user)
	}
	if user.Fields[2].Type != "*Profile" || user.Fields[3].Type != "[]Tag" {
		t.Fatalf("user fields = %#v", user.Fields)
	}

	repository := byName["example.com/shop/service.Repository"]
	if repository.Kind != model.Interface || len(repository.Extends) != 1 || repository.Extends[0].Name != "Finder" {
		t.Fatalf("repository = %#v", repository)
	}
	memory := byName["example.com/shop/service.MemoryRepository"]
	if !contains(memory.Implements, "example.com/shop/service.Repository") {
		t.Fatalf("memory repository implementations = %#v", memory.Implements)
	}
	service := byName["example.com/shop/service.UserService"]
	if len(service.Extends) != 1 || service.Extends[0].Name != "BaseService" {
		t.Fatalf("service embedding = %#v", service.Extends)
	}
	if !contains(service.Implements, "example.com/shop/service.Repository") {
		t.Fatalf("service implementations = %#v", service.Implements)
	}
}

func contains(values []model.TypeReference, want string) bool {
	for _, value := range values {
		if value.Name == want {
			return true
		}
	}
	return false
}

func TestParseSourceReportsSyntaxError(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
		line   int
		column int
		offset int
	}{
		{
			name:   "physical illegal declaration",
			source: "package broken\r\n//line virtual.go:800\r\ntype Valid struct{}\r\n@",
			line:   4, column: 1,
			offset: len("package broken\r\n//line virtual.go:800\r\ntype Valid struct{}\r\n"),
		},
		{
			name: "incomplete declaration", source: "package broken\ntype User struct {",
			line: 2, column: len("type User struct {") + 1,
			offset: len("package broken\ntype User struct {"),
		},
		{name: "empty file", line: 1, column: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			unit, err := ParseSource("broken.go", []byte(test.source))
			if err == nil {
				t.Fatal("syntax error was accepted")
			}
			if unit == nil {
				t.Fatalf("unit = %#v", unit)
			}
			if len(unit.Types) != 0 {
				t.Fatalf("syntax failure returned partial types: %#v", unit.Types)
			}
			if len(unit.Diagnostics) == 0 {
				t.Fatal("syntax failure returned no diagnostics")
			}
			want := model.SourcePosition{Offset: test.offset, Line: test.line, Column: test.column}
			for _, diagnostic := range unit.Diagnostics {
				if diagnostic.Code != "syntax-error" || diagnostic.Span.Start != want || diagnostic.Span.End != want {
					t.Errorf("diagnostic = %#v, want syntax-error at %#v", diagnostic, want)
				}
			}
		})
	}
}

func TestMethodsFromSeparateFilesAreMerged(t *testing.T) {
	declaration, err := ParseSource("user.go", []byte("package sample\n\ntype User struct {\nProfile *Profile\n}"))
	if err != nil {
		t.Fatal(err)
	}
	methods, err := ParseSource("user_methods.go", []byte("package sample\n//line redirected.go:800\nfunc (u *User) Name(\nprofile *Profile,\n) *Profile { return profile }"))
	if err != nil {
		t.Fatal(err)
	}
	otherMethods, err := ParseSource("user_save.go", []byte("package sample\nfunc (u *User) Save() {}"))
	if err != nil {
		t.Fatal(err)
	}
	if methods.Types[0].Line != 0 || otherMethods.Types[0].Line != 0 {
		t.Fatal("method-only synthetic types must not claim a declaration line")
	}
	for _, test := range []struct {
		name  string
		parts [][]model.Type
	}{
		{name: "declaration first", parts: [][]model.Type{declaration.Types, methods.Types, otherMethods.Types}},
		{name: "methods first", parts: [][]model.Type{methods.Types, otherMethods.Types, declaration.Types}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var parsed []model.Type
			for _, part := range test.parts {
				parsed = append(parsed, part...)
			}
			types := Finalize(parsed)
			if len(types) != 1 || len(types[0].Methods) != 2 {
				t.Fatalf("types = %#v", types)
			}
			user := types[0]
			if user.Source != "user.go" || user.Line != 3 {
				t.Fatalf("declaration = %s:%d, want user.go:3", user.Source, user.Line)
			}
			if len(user.Fields) != 1 || user.Fields[0].Source != "user.go" || user.Fields[0].Line != 4 {
				t.Fatalf("fields = %#v", user.Fields)
			}
			byName := map[string]model.Method{}
			for _, method := range user.Methods {
				byName[method.Name] = method
			}
			name := byName["Name"]
			if name.Source != "user_methods.go" || name.Line != 3 {
				t.Fatalf("Name declaration = %s:%d, want user_methods.go:3", name.Source, name.Line)
			}
			if len(name.Parameters) != 1 || name.Parameters[0].Line != 4 {
				t.Fatalf("Name parameters = %#v", name.Parameters)
			}
			save := byName["Save"]
			if save.Source != "user_save.go" || save.Line != 2 {
				t.Fatalf("Save declaration = %s:%d, want user_save.go:2", save.Source, save.Line)
			}
		})
	}
}

func TestEmbeddedInterfaceMethodsAreRequired(t *testing.T) {
	types := Finalize([]model.Type{
		{
			Package: "sample", Name: "Finder", Kind: model.Interface,
			Methods: []model.Method{{Name: "Find", ReturnType: "string"}},
		},
		{
			Package: "sample", Name: "Repository", Kind: model.Interface,
			Extends: []model.TypeReference{{Name: "Finder"}},
			Methods: []model.Method{{Name: "Save", ReturnType: "error"}},
		},
		{
			Package: "sample", Name: "SaveOnly", Kind: model.Struct,
			Methods: []model.Method{{Name: "Save", ReturnType: "error"}},
		},
	})
	for _, parsedType := range types {
		if parsedType.Name == "SaveOnly" && contains(parsedType.Implements, "sample.Repository") {
			t.Fatalf("SaveOnly must not implement Repository: %#v", parsedType.Implements)
		}
	}
}

func TestPointerReceiverMethods(t *testing.T) {
	// Test that pointer receiver methods are correctly handled
	types := Finalize([]model.Type{
		{
			Package: "sample", Name: "MyInterface", Kind: model.Interface,
			Methods: []model.Method{{Name: "DoSomething", ReturnType: "string"}},
		},
		{
			Package: "sample", Name: "ValueImpl", Kind: model.Struct,
			Methods: []model.Method{{Name: "DoSomething", ReturnType: "string", PointerReceiver: false}},
		},
		{
			Package: "sample", Name: "PointerImpl", Kind: model.Struct,
			Methods: []model.Method{{Name: "DoSomething", ReturnType: "string", PointerReceiver: true}},
		},
		{
			Package: "sample", Name: "MixedImpl", Kind: model.Struct,
			Methods: []model.Method{
				{Name: "DoSomething", ReturnType: "string", PointerReceiver: false},
				{Name: "AnotherMethod", PointerReceiver: true},
			},
		},
	})

	byName := map[string]model.Type{}
	for _, typ := range types {
		byName[typ.Name] = typ
	}

	// Check ValueImpl - should implement with value receiver
	valueImpl := byName["ValueImpl"]
	if len(valueImpl.Methods) != 1 {
		t.Fatalf("ValueImpl should have 1 method, got %d", len(valueImpl.Methods))
	}
	if valueImpl.Methods[0].PointerReceiver {
		t.Fatal("ValueImpl.DoSomething should not be a pointer receiver")
	}
	if !contains(valueImpl.Implements, "sample.MyInterface") {
		t.Fatalf("ValueImpl should implement MyInterface, got: %v", valueImpl.Implements)
	}

	// Check PointerImpl - should implement via pointer type
	pointerImpl := byName["PointerImpl"]
	if len(pointerImpl.Methods) != 1 {
		t.Fatalf("PointerImpl should have 1 method, got %d", len(pointerImpl.Methods))
	}
	if !pointerImpl.Methods[0].PointerReceiver {
		t.Fatal("PointerImpl.DoSomething should be a pointer receiver")
	}
	// Should still implement the interface (we check pointer type too)
	if !contains(pointerImpl.Implements, "sample.MyInterface") {
		t.Fatalf("PointerImpl should implement MyInterface (via pointer type), got: %v", pointerImpl.Implements)
	}

	// Check MixedImpl - should implement with value receiver method
	mixedImpl := byName["MixedImpl"]
	if len(mixedImpl.Methods) != 2 {
		t.Fatalf("MixedImpl should have 2 methods, got %d", len(mixedImpl.Methods))
	}
	// Find the DoSomething method
	var doSomethingMethod *model.Method
	for i := range mixedImpl.Methods {
		if mixedImpl.Methods[i].Name == "DoSomething" {
			doSomethingMethod = &mixedImpl.Methods[i]
			break
		}
	}
	if doSomethingMethod == nil {
		t.Fatal("MixedImpl should have DoSomething method")
	}
	if doSomethingMethod.PointerReceiver {
		t.Fatal("MixedImpl.DoSomething should not be a pointer receiver")
	}
	if !contains(mixedImpl.Implements, "sample.MyInterface") {
		t.Fatalf("MixedImpl should implement MyInterface, got: %v", mixedImpl.Implements)
	}
}

func TestPointerReceiverParsing(t *testing.T) {
	// Test that pointer receiver methods are correctly parsed from source
	source := []byte(`
package testpkg

type MyStruct struct{}

func (m MyStruct) ValueMethod() {}
func (m *MyStruct) PointerMethod() {}
`)
	unit, err := ParseSource("test.go", source)
	if err != nil {
		t.Fatal(err)
	}
	types := unit.Types

	if len(types) != 1 {
		t.Fatalf("Expected 1 type, got %d", len(types))
	}

	myStruct := types[0]
	if myStruct.Name != "MyStruct" {
		t.Fatalf("Expected MyStruct, got %s", myStruct.Name)
	}

	if len(myStruct.Methods) != 2 {
		t.Fatalf("Expected 2 methods, got %d", len(myStruct.Methods))
	}

	// Find methods by name
	var valueMethod, pointerMethod *model.Method
	for i := range myStruct.Methods {
		if myStruct.Methods[i].Name == "ValueMethod" {
			valueMethod = &myStruct.Methods[i]
		} else if myStruct.Methods[i].Name == "PointerMethod" {
			pointerMethod = &myStruct.Methods[i]
		}
	}

	if valueMethod == nil {
		t.Fatal("ValueMethod not found")
	}
	if pointerMethod == nil {
		t.Fatal("PointerMethod not found")
	}

	if valueMethod.PointerReceiver {
		t.Fatal("ValueMethod should not be a pointer receiver")
	}
	if !pointerMethod.PointerReceiver {
		t.Fatal("PointerMethod should be a pointer receiver")
	}
}

func TestPhysicalDeclarationAndReferenceLocations(t *testing.T) {
	const path = "locations.go"
	source := []byte(`package sample
//line redirected.go:900
type Base interface {
	Find(
		item *Item,
	) *Item
}
type Contract interface {
	Base
}
type Embedded struct{}
type Candidate struct {
	Embedded
	*Other
	Item *Item
}
func (c Candidate) Find(
	item *Item,
) *Item { return item }
type Alias *Item
type Unnamed interface {
	Take(
		*Item,
	)
}
`)
	parsed, err := ParseSource(path, source)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]model.Type{}
	for _, typ := range Finalize(parsed.Types) {
		byName[typ.Name] = typ
	}
	for name, line := range map[string]int{
		"Base": 3, "Contract": 8, "Embedded": 11, "Candidate": 12, "Alias": 20, "Unnamed": 21,
	} {
		typ := byName[name]
		if typ.Source != path || typ.Line != line {
			t.Errorf("%s declaration = %s:%d, want %s:%d", name, typ.Source, typ.Line, path, line)
		}
	}
	base := byName["Base"]
	if len(base.Methods) != 1 || base.Methods[0].Source != path || base.Methods[0].Line != 4 {
		t.Fatalf("Base methods = %#v", base.Methods)
	}
	if len(base.Methods[0].Parameters) != 1 || base.Methods[0].Parameters[0].Line != 5 {
		t.Fatalf("Base.Find parameters = %#v", base.Methods[0].Parameters)
	}
	contract := byName["Contract"]
	if len(contract.Extends) != 1 || contract.Extends[0] != (model.TypeReference{Name: "Base", Source: path, Line: 9}) {
		t.Fatalf("Contract parents = %#v", contract.Extends)
	}
	candidate := byName["Candidate"]
	if len(candidate.Extends) != 2 ||
		candidate.Extends[0] != (model.TypeReference{Name: "Embedded", Source: path, Line: 13}) ||
		candidate.Extends[1] != (model.TypeReference{Name: "*Other", Source: path, Line: 14}) {
		t.Fatalf("Candidate parents = %#v", candidate.Extends)
	}
	if len(candidate.Fields) != 1 || candidate.Fields[0].Source != path || candidate.Fields[0].Line != 15 {
		t.Fatalf("Candidate fields = %#v", candidate.Fields)
	}
	if len(candidate.Methods) != 1 || candidate.Methods[0].Source != path || candidate.Methods[0].Line != 17 {
		t.Fatalf("Candidate methods = %#v", candidate.Methods)
	}
	if len(candidate.Methods[0].Parameters) != 1 || candidate.Methods[0].Parameters[0].Line != 18 {
		t.Fatalf("Candidate.Find parameters = %#v", candidate.Methods[0].Parameters)
	}
	if len(candidate.Implements) != 2 ||
		candidate.Implements[0] != (model.TypeReference{Name: base.QualifiedName(), Source: path, Line: 12, Inferred: true}) ||
		candidate.Implements[1] != (model.TypeReference{Name: contract.QualifiedName(), Source: path, Line: 12, Inferred: true}) {
		t.Fatalf("Candidate implementations = %#v", candidate.Implements)
	}
	alias := byName["Alias"]
	if len(alias.Fields) != 1 || alias.Fields[0].Source != path || alias.Fields[0].Line != 20 {
		t.Fatalf("Alias fields = %#v", alias.Fields)
	}
	unnamed := byName["Unnamed"]
	if len(unnamed.Methods) != 1 || unnamed.Methods[0].Source != path || unnamed.Methods[0].Line != 22 {
		t.Fatalf("Unnamed methods = %#v", unnamed.Methods)
	}
	if len(unnamed.Methods[0].Parameters) != 1 || unnamed.Methods[0].Parameters[0].Line != 23 {
		t.Fatalf("Unnamed.Take parameters = %#v", unnamed.Methods[0].Parameters)
	}
}

func TestInferredImplementationUsesMethodLocationWithoutDeclaration(t *testing.T) {
	contracts, err := ParseSource("contract.go", []byte("package sample\ntype Contract interface { Find() }"))
	if err != nil {
		t.Fatal(err)
	}
	methods, err := ParseSource("candidate_methods.go", []byte("package sample\n//line redirected.go:500\nfunc (c Candidate) Find() {}"))
	if err != nil {
		t.Fatal(err)
	}
	declarations, err := ParseSource("candidate.go", []byte("package sample\n\ntype Candidate struct{}"))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name         string
		declarations []model.Type
		source       string
		line         int
	}{
		{name: "method only", source: "candidate_methods.go", line: 3},
		{name: "real declaration", declarations: declarations.Types, source: "candidate.go", line: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			parsed := append([]model.Type(nil), methods.Types...)
			parsed = append(parsed, contracts.Types...)
			parsed = append(parsed, test.declarations...)
			types := Finalize(parsed)
			var candidate, contract model.Type
			for _, typ := range types {
				if typ.Name == "Candidate" {
					candidate = typ
				}
				if typ.Name == "Contract" {
					contract = typ
				}
			}
			want := model.TypeReference{
				Name: contract.QualifiedName(), Source: test.source, Line: test.line, Inferred: true,
			}
			if len(candidate.Implements) != 1 || candidate.Implements[0] != want {
				t.Fatalf("Candidate implementations = %#v, want %#v", candidate.Implements, want)
			}
			if test.declarations == nil && candidate.Line != 0 {
				t.Fatalf("synthetic declaration line = %d, want 0", candidate.Line)
			}
		})
	}
}

func TestFinalizeWithEvidenceUsesMatchedMethodDeclarations(t *testing.T) {
	var parsed []model.Type
	for _, file := range []struct {
		path   string
		source string
	}{
		{
			path: "contract.go",
			source: "package sample\n" +
				"type Contract interface {\n" +
				" Find(id int) string\n" +
				" Save(value string) error\n" +
				"}\n",
		},
		{
			path: "candidate_methods.go",
			source: "package sample\n" +
				"//line redirected.go:900\n" +
				"func (c Candidate) Save(value string) error { return nil }\n" +
				"func (c Candidate) Unused() {}\n",
		},
		{
			path: "embedded_methods.go",
			source: "package sample\n" +
				"//line redirected.go:800\n" +
				"func (e *Embedded) Find(id int) string { return \"\" }\n",
		},
		{
			path: "candidate.go",
			source: "package sample\n" +
				"type Embedded struct{}\n" +
				"type Candidate struct { Embedded }\n",
		},
	} {
		unit, err := ParseSource(file.path, []byte(file.source))
		if err != nil {
			t.Fatal(err)
		}
		parsed = append(parsed, unit.Types...)
	}

	ordinary := Finalize(parsed)
	explained := FinalizeWithEvidence(parsed)
	withoutEvidence := append([]model.Type(nil), explained...)
	var candidate, contract model.Type
	for index, typ := range explained {
		withoutEvidence[index].ImplementationMethods = nil
		if typ.Name == "Candidate" {
			candidate = typ
		}
		if typ.Name == "Contract" {
			contract = typ
		}
	}
	if !reflect.DeepEqual(withoutEvidence, ordinary) {
		t.Fatalf("evidence finalization changed ordinary model:\n got %#v\nwant %#v", withoutEvidence, ordinary)
	}
	wantReference := model.TypeReference{
		Name: contract.QualifiedName(), Source: "candidate.go", Line: 3, Inferred: true,
	}
	if !reflect.DeepEqual(candidate.Implements, []model.TypeReference{wantReference}) {
		t.Fatalf("Candidate implementations = %#v, want %#v", candidate.Implements, wantReference)
	}
	wantMethods := []model.ImplementationMethod{
		{Interface: contract.QualifiedName(), Signature: "Find(int)string", Source: "embedded_methods.go", Line: 3},
		{Interface: contract.QualifiedName(), Signature: "Save(string)error", Source: "candidate_methods.go", Line: 3},
	}
	if !reflect.DeepEqual(candidate.ImplementationMethods, wantMethods) {
		t.Fatalf("Candidate method evidence = %#v, want %#v", candidate.ImplementationMethods, wantMethods)
	}

	// Derived evidence must be rebuilt, including after an explanation result is
	// reused as input, and must not survive a return to ordinary finalization.
	stale := append([]model.Type(nil), explained...)
	for index := range stale {
		stale[index].ImplementationMethods = []model.ImplementationMethod{
			{Interface: "stale.Interface", Signature: "Old()", Source: "old.go", Line: 99},
		}
	}
	if rebuilt := FinalizeWithEvidence(stale); !reflect.DeepEqual(rebuilt, explained) {
		t.Fatalf("rebuilt evidence = %#v, want %#v", rebuilt, explained)
	}
	if cleared := Finalize(stale); !reflect.DeepEqual(cleared, ordinary) {
		t.Fatalf("ordinary finalization retained derived evidence: %#v", cleared)
	}
}

func TestFunctionsOnlySourceUnitRetainsPackageAndImports(t *testing.T) {
	source := []byte("package sample\nimport \"fmt\"\nfunc Run() { fmt.Println(\"ok\") }\n")
	unit, err := ParseSource("functions.go", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(unit.Types) != 0 || len(unit.Diagnostics) != 0 {
		t.Fatalf("function-only declarations = %#v, diagnostics = %#v", unit.Types, unit.Diagnostics)
	}
	wantPackage := model.PackageDeclaration{
		Name: "sample",
		Span: model.SourceSpan{
			Start: model.SourcePosition{Offset: 0, Line: 1, Column: 1},
			End:   model.SourcePosition{Offset: 14, Line: 1, Column: 15},
		},
	}
	if unit.Package != wantPackage {
		t.Fatalf("package = %#v, want %#v", unit.Package, wantPackage)
	}
	wantImports := []model.Import{{
		Name: "fmt",
		Span: model.SourceSpan{
			Start: model.SourcePosition{Offset: 22, Line: 2, Column: 8},
			End:   model.SourcePosition{Offset: 27, Line: 2, Column: 13},
		},
	}}
	if !reflect.DeepEqual(unit.Imports, wantImports) {
		t.Fatalf("imports = %#v, want %#v", unit.Imports, wantImports)
	}
	wantSpan := model.SourceSpan{
		Start: model.SourcePosition{Offset: 0, Line: 1, Column: 1},
		End:   model.SourcePosition{Offset: len(source), Line: 4, Column: 1},
	}
	if unit.Span != wantSpan {
		t.Fatalf("file span = %#v, want %#v", unit.Span, wantSpan)
	}
}

func TestSourceUnitPhysicalImportSpans(t *testing.T) {
	source := "//line virtual.go:900\r\npackage café\r\n//line virtual.go:1200\r\n" +
		"import (\r\n\t\"fmt\"\r\n\ttexte \"example.com/é\"\r\n\t. \"math\"\r\n\t_ \"embed\"\r\n)\r\nfunc rôle() {}"
	unit, err := ParseSource("physical.go", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	packageOffset := strings.Index(source, "package")
	wantPackage := model.PackageDeclaration{
		Name: "café",
		Span: model.SourceSpan{
			Start: model.SourcePosition{Offset: packageOffset, Line: 2, Column: 1},
			End:   model.SourcePosition{Offset: packageOffset + len("package café"), Line: 2, Column: len("package café") + 1},
		},
	}
	if unit.Package != wantPackage {
		t.Fatalf("package = %#v, want %#v", unit.Package, wantPackage)
	}
	wantEnd := model.SourcePosition{Offset: len(source), Line: 10, Column: len("func rôle() {}") + 1}
	if unit.Span.Start != (model.SourcePosition{Line: 1, Column: 1}) || unit.Span.End != wantEnd {
		t.Fatalf("file span = %#v, want physical end %#v", unit.Span, wantEnd)
	}
	specs := []struct {
		text  string
		name  string
		alias string
		line  int
	}{
		{text: `"fmt"`, name: "fmt", line: 5},
		{text: `texte "example.com/é"`, name: "example.com/é", alias: "texte", line: 6},
		{text: `. "math"`, name: "math", alias: ".", line: 7},
		{text: `_ "embed"`, name: "embed", alias: "_", line: 8},
	}
	if len(unit.Imports) != len(specs) {
		t.Fatalf("imports = %#v", unit.Imports)
	}
	for i, spec := range specs {
		offset := strings.Index(source, spec.text)
		want := model.Import{
			Name: spec.name, Alias: spec.alias,
			Span: model.SourceSpan{
				Start: model.SourcePosition{Offset: offset, Line: spec.line, Column: 2},
				End:   model.SourcePosition{Offset: offset + len(spec.text), Line: spec.line, Column: len(spec.text) + 2},
			},
		}
		if unit.Imports[i] != want {
			t.Errorf("import %d = %#v, want %#v", i, unit.Imports[i], want)
		}
	}
}

func TestSourceUnitsKeepIndependentCanonicalImportsBeforeFinalize(t *testing.T) {
	left, err := ParseSource("left.go", []byte("package sample\nimport dep \"example.com/left\"\ntype User struct { Value dep.Value }\ntype Other struct{}\nfunc (u User) Left(v dep.Value) {}"))
	if err != nil {
		t.Fatal(err)
	}
	right, err := ParseSource("right.go", []byte("package sample\nimport dep \"example.com/right\"\nfunc (u User) Right(v dep.Value) {}"))
	if err != nil {
		t.Fatal(err)
	}
	for _, unit := range []*model.SourceUnit{left, right} {
		for _, typ := range unit.Types {
			if !reflect.DeepEqual(typ.Imports, unit.Imports) {
				t.Fatalf("%s.%s lost its file's imports", unit.Path, typ.Name)
			}
		}
	}
	if len(left.Imports) != 1 || len(right.Imports) != 1 ||
		left.Imports[0].Name != "example.com/left" || right.Imports[0].Name != "example.com/right" ||
		left.Imports[0].Alias != "dep" || right.Imports[0].Alias != "dep" {
		t.Fatalf("independent unit imports: left = %#v, right = %#v", left.Imports, right.Imports)
	}
	var leftUser model.Type
	for _, typ := range left.Types {
		if typ.Name == "User" {
			leftUser = typ
		}
	}
	if len(leftUser.Methods) != 1 || leftUser.Methods[0].Parameters[0].Type != "dep.Value" {
		t.Fatalf("left methods = %#v", leftUser.Methods)
	}
	if len(right.Types) != 1 || right.Types[0].Name != "User" || len(right.Types[0].Methods) != 1 ||
		right.Types[0].Methods[0].Parameters[0].Type != "dep.Value" {
		t.Fatalf("right types = %#v", right.Types)
	}
}

func TestFinalizeDoesNotAppendIntoSourceUnitImports(t *testing.T) {
	for _, finalize := range []struct {
		name string
		run  func([]model.Type) []model.Type
	}{
		{name: "ordinary", run: Finalize},
		{name: "evidence", run: FinalizeWithEvidence},
	} {
		t.Run(finalize.name, func(t *testing.T) {
			left, err := ParseSource("left.go", []byte("package sample\nimport left \"example.com/left\"\ntype User struct{}"))
			if err != nil {
				t.Fatal(err)
			}
			right, err := ParseSource("right.go", []byte("package sample\nimport right \"example.com/right\"\nfunc (u User) Right() {}"))
			if err != nil {
				t.Fatal(err)
			}
			repeated, err := ParseSource("extra.go", []byte("package sample\n\nimport right \"example.com/right\"\nfunc (u User) Extra() {}"))
			if err != nil {
				t.Fatal(err)
			}
			// Reserve capacity so appending a file-scoped import would overwrite
			// the class projection's merged dependency if ownership is lost.
			imports := make([]model.Import, len(left.Imports), len(left.Imports)+1)
			copy(imports, left.Imports)
			left.Imports = imports
			left.RebindPath(left.Path)
			types := append(append(append([]model.Type(nil), left.Types...), right.Types...), repeated.Types...)
			projected := finalize.run(types)
			if len(projected) != 1 || len(projected[0].Imports) != 2 {
				t.Fatalf("merged imports must ignore declaration positions: %#v", projected)
			}
			left.Imports = append(left.Imports, model.Import{Name: "example.com/new"})
			if projected[0].Imports[0].Name != "example.com/left" ||
				projected[0].Imports[1].Name != "example.com/right" ||
				len(right.Imports) != 1 || len(repeated.Imports) != 1 {
				t.Fatalf("file import change corrupted the class projection: %#v", projected)
			}
		})
	}
}

func TestBindContextRebindsUnitsIncludingFilesWithoutTypes(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	second := filepath.Join(root, "second")
	for _, directory := range []string{first, second} {
		if err := os.MkdirAll(filepath.Join(directory, "nested"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, module := range map[string]string{
		filepath.Join(first, "go.mod"):  "module example.com/first\n",
		filepath.Join(second, "go.mod"): "module example.com/second\n",
	} {
		if err := os.WriteFile(path, []byte(module), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name   string
		source string
	}{
		{name: "functions only", source: "package declared\nfunc Run() {}"},
		{name: "type declaration", source: "package declared\ntype Value struct{}"},
		{name: "package only", source: "package declared"},
	} {
		t.Run(test.name, func(t *testing.T) {
			unit, err := ParseSource(filepath.Join(first, "nested", "file.go"), []byte(test.source))
			if err != nil {
				t.Fatal(err)
			}
			if unit.PackagePath != "" {
				t.Fatalf("parse bound module context: %q", unit.PackagePath)
			}
			for _, typ := range unit.Types {
				if typ.Package != "declared" {
					t.Fatalf("parsed package = %q, want declared", typ.Package)
				}
			}
			for _, binding := range []struct {
				path string
				want string
			}{
				{path: filepath.Join(first, "nested", "file.go"), want: "example.com/first/nested"},
				{path: filepath.Join(second, "file.go"), want: "example.com/second"},
				{path: filepath.Join(root, "standalone.go"), want: "declared"},
			} {
				unit.RebindPath(binding.path)
				BindContext(unit)
				if unit.Package.Name != "declared" || unit.PackagePath != binding.want {
					t.Fatalf("bound package = %#v, package path = %q, want %q", unit.Package, unit.PackagePath, binding.want)
				}
				for _, typ := range unit.Types {
					if typ.Package != binding.want {
						t.Fatalf("type package = %q, want %q", typ.Package, binding.want)
					}
				}
			}
		})
	}
}
