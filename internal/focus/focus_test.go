package focus

import (
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/model"
)

func TestApplyByDepth(t *testing.T) {
	types := []model.Type{
		{Package: "sample", Name: "Controller", Fields: []model.Field{{Type: "Service"}}},
		{Package: "sample", Name: "Service", Fields: []model.Field{{Type: "Repository"}}},
		{Package: "sample", Name: "Repository", Methods: []model.Method{{ReturnType: "User"}}},
		{Package: "sample", Name: "User"},
		{Package: "sample", Name: "Unrelated"},
	}
	got, err := Apply(types, "Service", 1)
	if err != nil {
		t.Fatal(err)
	}
	if names(got) != "Controller,Service,Repository" {
		t.Fatalf("depth 1 = %s", names(got))
	}
	got, err = Apply(types, "sample.Service", 2)
	if err != nil {
		t.Fatal(err)
	}
	if names(got) != "Controller,Service,Repository,User" {
		t.Fatalf("depth 2 = %s", names(got))
	}
}

func TestApplyReportsAmbiguousSimpleName(t *testing.T) {
	types := []model.Type{{Package: "one", Name: "User"}, {Package: "two", Name: "User"}}
	_, err := Apply(types, "User", 1)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("err = %v", err)
	}
}

func names(types []model.Type) string {
	var result []string
	for _, t := range types {
		result = append(result, t.Name)
	}
	return strings.Join(result, ",")
}
