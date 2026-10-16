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
	if repository.Kind != model.Interface || len(repository.Extends) != 1 || repository.Extends[0] != "Finder" {
		t.Fatalf("repository = %#v", repository)
	}
	memory := byName["example.com/shop/service.MemoryRepository"]
	if !contains(memory.Implements, "example.com/shop/service.Repository") {
		t.Fatalf("memory repository implementations = %#v", memory.Implements)
	}
	service := byName["example.com/shop/service.UserService"]
	if len(service.Extends) != 1 || service.Extends[0] != "BaseService" {
		t.Fatalf("service embedding = %#v", service.Extends)
	}
	if !contains(service.Implements, "example.com/shop/service.Repository") {
		t.Fatalf("service implementations = %#v", service.Implements)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
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
	declaration, err := ParseSource("user.go", []byte("package sample\ntype User struct{}"))
	if err != nil {
		t.Fatal(err)
	}
	methods, err := ParseSource("user_methods.go", []byte("package sample\nfunc (u *User) Name() string { return \"\" }"))
	if err != nil {
		t.Fatal(err)
	}
	types := Finalize(append(declaration, methods...))
	if len(types) != 1 || len(types[0].Methods) != 1 || types[0].Methods[0].Name != "Name" {
		t.Fatalf("types = %#v", types)
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
			Extends: []string{"Finder"},
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
