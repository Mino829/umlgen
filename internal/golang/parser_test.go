package golang

import (
	"path/filepath"
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
		types, parseErr := ParseFile(file)
		if parseErr != nil {
			t.Fatalf("ParseFile(%s): %v", file, parseErr)
		}
		parsed = append(parsed, types...)
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
	_, err := ParseSource("broken.go", []byte("package broken\ntype User struct {"))
	if err == nil {
		t.Fatal("expected syntax error")
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
	if methods[0].Line != 0 || otherMethods[0].Line != 0 {
		t.Fatal("method-only synthetic types must not claim a declaration line")
	}
	for _, test := range []struct {
		name  string
		parts [][]model.Type
	}{
		{name: "declaration first", parts: [][]model.Type{declaration, methods, otherMethods}},
		{name: "methods first", parts: [][]model.Type{methods, otherMethods, declaration}},
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
	types, err := ParseSource("test.go", source)
	if err != nil {
		t.Fatal(err)
	}

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
	for _, typ := range Finalize(parsed) {
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
		{name: "real declaration", declarations: declarations, source: "candidate.go", line: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			parsed := append([]model.Type(nil), methods...)
			parsed = append(parsed, contracts...)
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
