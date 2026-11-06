// Package sequencego analyzes only supplied Go source, using go/types without
// loading dependencies or executing build tools. It models lexical call/receive
// ordering, not non-call operand order, runtime values, or implicit initialization.
package sequencego

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Mino829/umlgen/internal/golang"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/sequence"
)

type sourceFile struct {
	source     sequence.Source
	ast        *ast.File
	pkg        *sourcePackage
	parseError error
}
type sourcePackage struct {
	key, path, context string
	files              []*sourceFile
	info               *types.Info
	checked            *types.Package
	state              int
}
type declaration struct {
	callable sequence.Callable
	node     ast.Node
	body     *ast.BlockStmt
	values   []ast.Expr
	object   *types.Func
	pkg      *sourcePackage
}
type analyzer struct {
	ctx          context.Context
	fset         *token.FileSet
	program      sequence.Program
	files        []*sourceFile
	sourceByPath map[string]sequence.Source
	packages     map[string]*sourcePackage
	imports      map[string][]*sourcePackage
	declarations []*declaration
	functions    map[*types.Func]*declaration
	literals     map[*ast.FuncLit]*declaration
}

// Analyze performs fresh, in-process semantic analysis of the selected snapshots.
// Parse/type errors are diagnostics on a partial program; cancellation is an error.
func Analyze(ctx context.Context, sources []sequence.Source) (sequence.Program, error) {
	a := &analyzer{ctx: ctx, fset: token.NewFileSet(), program: sequence.Program{Language: "go", Backend: "go/parser + go/types (selected sources only)"}, packages: map[string]*sourcePackage{}, imports: map[string][]*sourcePackage{}, functions: map[*types.Func]*declaration{}, literals: map[*ast.FuncLit]*declaration{}}
	a.sourceByPath = make(map[string]sequence.Source, len(sources))
	ordered := append([]sequence.Source(nil), sources...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	seen := map[string]bool{}
	for _, source := range ordered {
		if err := ctx.Err(); err != nil {
			return a.program, err
		}
		if seen[source.Path] {
			a.program.Diagnostics = append(a.program.Diagnostics, sequence.Diagnostic{Code: "duplicate-source", Message: "duplicate selected source path", File: source.Path, Error: true})
			continue
		}
		seen[source.Path] = true
		a.sourceByPath[source.Path] = source
		hash := sha256.Sum256(source.Content)
		a.program.Files = append(a.program.Files, sequence.File{Path: source.Path, Hash: fmt.Sprintf("%x", hash)})
		file, err := parser.ParseFile(a.fset, source.Path, source.Content, parser.AllErrors|parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			a.parseErrors(err, source)
		}
		if file == nil || file.Name == nil {
			continue
		}
		unit := &model.SourceUnit{Path: source.Path, Package: model.PackageDeclaration{Name: file.Name.Name}}
		golang.BindContext(unit)
		key := filepath.Dir(source.Path) + "\x00" + file.Name.Name
		pkg := a.packages[key]
		if pkg == nil {
			pkg = &sourcePackage{key: key, path: unit.PackagePath, context: unit.PackageContext, info: &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}, Instances: map[*ast.Ident]types.Instance{}, Scopes: map[ast.Node]*types.Scope{}}}
			a.packages[key] = pkg
			a.imports[pkg.path] = append(a.imports[pkg.path], pkg)
		}
		sf := &sourceFile{source: source, ast: file, pkg: pkg, parseError: err}
		pkg.files = append(pkg.files, sf)
		a.files = append(a.files, sf)
	}
	keys := make([]string, 0, len(a.packages))
	for key := range a.packages {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return a.program, err
		}
		a.check(a.packages[key])
	}
	for _, sf := range a.files {
		a.collect(sf)
	}
	sort.Slice(a.declarations, func(i, j int) bool { return a.declarations[i].callable.ID < a.declarations[j].callable.ID })
	ids := map[string]bool{}
	for _, d := range a.declarations {
		if err := ctx.Err(); err != nil {
			return a.program, err
		}
		if ids[d.callable.ID] {
			a.diagnostic("duplicate-callable", "duplicate callable identity; declaration omitted", d.node, true)
			continue
		}
		ids[d.callable.ID] = true
		b := builder{a: a, pkg: d.pkg}
		if d.body != nil {
			d.callable.Body, _ = b.block(d.body.List)
		} else {
			for _, v := range d.values {
				d.callable.Body = append(d.callable.Body, b.expr(v)...)
			}
		}
		if d.body == nil && d.values == nil {
			d.callable.Body = []sequence.Event{b.unsupported(d.node, "callable has no selected source body")}
		}
		var markers []sequence.Event
		for _, diagnostic := range a.program.Diagnostics {
			if diagnostic.Error && (diagnostic.Code == "type-error" || diagnostic.Code == "parse-error") && diagnostic.File == d.callable.File && diagnostic.Span.Start.Offset >= d.callable.Span.Start.Offset && diagnostic.Span.Start.Offset < d.callable.Span.End.Offset {
				markers = append(markers, sequence.Event{Kind: sequence.Note, Label: diagnostic.Code + ": " + diagnostic.Message, File: diagnostic.File, Span: diagnostic.Span})
			}
		}
		d.callable.Body = append(markers, d.callable.Body...)
		a.program.Callables = append(a.program.Callables, d.callable)
	}
	sort.SliceStable(a.program.Diagnostics, func(i, j int) bool {
		x, y := a.program.Diagnostics[i], a.program.Diagnostics[j]
		if x.File != y.File {
			return x.File < y.File
		}
		if x.Span.Start.Offset != y.Span.Start.Offset {
			return x.Span.Start.Offset < y.Span.Start.Offset
		}
		if x.Code != y.Code {
			return x.Code < y.Code
		}
		return x.Message < y.Message
	})
	return a.program, ctx.Err()
}

func (a *analyzer) Import(path string) (*types.Package, error) {
	if err := a.ctx.Err(); err != nil {
		return nil, err
	}
	if path == "unsafe" {
		return types.Unsafe, nil
	}
	candidates := a.imports[path]
	if len(candidates) != 1 {
		if len(candidates) > 1 {
			return nil, fmt.Errorf("selected import %q is ambiguous across source packages", path)
		}
		return nil, fmt.Errorf("import %q is not available in selected sources (external source loading disabled)", path)
	}
	p := candidates[0]
	if p.state == 1 {
		return nil, fmt.Errorf("selected-source import cycle involving %q", path)
	}
	a.check(p)
	if p.checked == nil {
		return nil, fmt.Errorf("selected package %q could not be checked", path)
	}
	return p.checked, nil
}
func (a *analyzer) check(p *sourcePackage) {
	if p.state != 0 {
		return
	}
	p.state = 1
	files := make([]*ast.File, 0, len(p.files))
	for _, sf := range p.files {
		files = append(files, sf.ast)
	}
	config := types.Config{Importer: a, Error: func(err error) {
		if e, ok := err.(types.Error); ok {
			a.diagnosticAt("type-error", e.Msg, e.Pos, true)
		} else {
			a.program.Diagnostics = append(a.program.Diagnostics, sequence.Diagnostic{Code: "type-error", Message: err.Error(), Error: true})
		}
	}}
	p.checked, _ = config.Check(p.path, a.fset, files, p.info)
	p.state = 2
}
func (a *analyzer) parseErrors(err error, source sequence.Source) {
	if list, ok := err.(scanner.ErrorList); ok {
		for _, e := range list {
			offset := e.Pos.Offset
			if offset < 0 {
				offset = 0
			}
			if offset > len(source.Content) {
				offset = len(source.Content)
			}
			prefix := source.Content[:offset]
			pos := model.SourcePosition{Offset: offset, Line: bytes.Count(prefix, []byte{'\n'}) + 1, Column: offset - bytes.LastIndexByte(prefix, '\n')}
			a.program.Diagnostics = append(a.program.Diagnostics, sequence.Diagnostic{Code: "parse-error", Message: e.Msg, File: source.Path, Span: model.SourceSpan{Start: pos, End: pos}, Error: true})
		}
		return
	}
	a.program.Diagnostics = append(a.program.Diagnostics, sequence.Diagnostic{Code: "parse-error", Message: err.Error(), File: source.Path, Error: true})
}
func (a *analyzer) position(pos token.Pos) model.SourcePosition {
	p := a.fset.PositionFor(pos, false)
	return model.SourcePosition{Offset: p.Offset, Line: p.Line, Column: p.Column}
}
func (a *analyzer) location(n ast.Node) (string, model.SourceSpan) {
	if n == nil {
		return "", model.SourceSpan{}
	}
	return a.fset.PositionFor(n.Pos(), false).Filename, model.SourceSpan{Start: a.position(n.Pos()), End: a.position(n.End())}
}
func (a *analyzer) diagnostic(code, message string, n ast.Node, bad bool) {
	file, span := a.location(n)
	a.program.Diagnostics = append(a.program.Diagnostics, sequence.Diagnostic{Code: code, Message: message, File: file, Span: span, Error: bad})
}
func (a *analyzer) diagnosticAt(code, message string, pos token.Pos, bad bool) {
	p := a.position(pos)
	a.program.Diagnostics = append(a.program.Diagnostics, sequence.Diagnostic{Code: code, Message: message, File: a.fset.PositionFor(pos, false).Filename, Span: model.SourceSpan{Start: p, End: p}, Error: bad})
}
func typeName(t types.Type) string {
	if t == nil {
		return "<?>"
	}
	return types.TypeString(t, func(p *types.Package) string { return p.Path() })
}
func signatureName(sig *types.Signature) string {
	parts := make([]string, sig.Params().Len())
	for i := range parts {
		t := sig.Params().At(i).Type()
		if sig.Variadic() && i == len(parts)-1 {
			if s, ok := t.(*types.Slice); ok {
				parts[i] = "..." + typeName(s.Elem())
				continue
			}
		}
		parts[i] = typeName(t)
	}
	return "(" + strings.Join(parts, ",") + ")"
}
func returnsName(sig *types.Signature) string {
	if sig.Results().Len() == 0 {
		return ""
	}
	parts := make([]string, sig.Results().Len())
	for i := range parts {
		parts[i] = typeName(sig.Results().At(i).Type())
	}
	return strings.Join(parts, ", ")
}

// Parser recovery can fabricate signatures, expressions and closing delimiters.
// Do not expose a damaged region (or its nested literals) as a callable body.
func (a *analyzer) parserDamaged(sf *sourceFile, node ast.Node) bool {
	if sf.parseError == nil {
		return false
	}
	// BlockStmt.End may fall back to its last statement when '}' is absent,
	// leaving the EOF diagnostic outside the recovered node's reported span.
	switch node := node.(type) {
	case *ast.FuncDecl:
		if node.Body != nil && !node.Body.Rbrace.IsValid() {
			return true
		}
	case *ast.FuncLit:
		if node.Body == nil || !node.Body.Rbrace.IsValid() {
			return true
		}
	case *ast.ValueSpec:
		for _, value := range node.Values {
			if a.parserDamaged(sf, value) {
				return true
			}
		}
	case *ast.ParenExpr:
		if a.parserDamaged(sf, node.X) {
			return true
		}
	}
	issues, ok := sf.parseError.(scanner.ErrorList)
	if !ok {
		return true
	}
	_, span := a.location(node)
	if span.Start.Line == 0 || span.End.Line == 0 || span.End.Offset < span.Start.Offset {
		return true
	}
	for _, issue := range issues {
		if issue.Pos.Offset >= span.Start.Offset && issue.Pos.Offset <= span.End.Offset {
			return true
		}
	}
	return false
}

func (a *analyzer) collect(sf *sourceFile) {
	for _, node := range sf.ast.Decls {
		if fn, ok := node.(*ast.FuncDecl); ok {
			if fn.Body == nil || a.parserDamaged(sf, fn) {
				continue
			} // A missing or parser-damaged body is not an executable entry.
			obj, _ := sf.pkg.info.Defs[fn.Name].(*types.Func)
			qualified := sf.pkg.path + "." + fn.Name.Name
			params := "(?)"
			returns := ""
			participant := sf.pkg.path
			if obj != nil {
				sig := obj.Type().(*types.Signature)
				params = signatureName(sig)
				returns = returnsName(sig)
				if sig.Recv() != nil {
					recv := typeName(sig.Recv().Type())
					short := strings.TrimPrefix(recv, sf.pkg.path+".")
					short = strings.TrimPrefix(short, "*"+sf.pkg.path+".")
					if strings.HasPrefix(recv, "*") {
						short = "*" + short
					}
					qualified = sf.pkg.path + ".(" + short + ")." + fn.Name.Name
					participant = recv
				}
			} else if fn.Recv != nil {
				qualified = sf.pkg.path + ".(unresolved@" + a.suffix(fn) + ")." + fn.Name.Name
			}
			if fn.Name.Name == "init" {
				qualified += "@" + a.suffix(fn)
			}
			d := a.declare(sf, fn, fn.Body, nil, fn.Name.Name, qualified, participant, params, returns, obj)
			if obj != nil {
				a.functions[obj.Origin()] = d
			}
		}
		if gen, ok := node.(*ast.GenDecl); ok && gen.Tok == token.VAR {
			for _, spec := range gen.Specs {
				if v, ok := spec.(*ast.ValueSpec); ok && len(v.Values) > 0 && !a.parserDamaged(sf, v) {
					a.declare(sf, v, nil, v.Values, "<initializer>", sf.pkg.path+".<initializer>@"+a.suffix(v), sf.pkg.path, "()", "", nil)
					a.diagnostic("implicit-initialization", "Package variable initializer is a separate source region; implicit package initialization order is not projected.", v, false)
				}
			}
		}
	}
	ast.Inspect(sf.ast, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.FuncDecl, *ast.ValueSpec, *ast.FuncLit:
			if a.parserDamaged(sf, n) {
				return false
			}
		}
		if lit, ok := n.(*ast.FuncLit); ok {
			sig, _ := sf.pkg.info.TypeOf(lit).(*types.Signature)
			params, returns := "(?)", ""
			if sig != nil {
				params = signatureName(sig)
				returns = returnsName(sig)
			}
			d := a.declare(sf, lit, lit.Body, nil, "func", sf.pkg.path+".func@"+a.suffix(lit), sf.pkg.path, params, returns, nil)
			a.literals[lit] = d
		}
		return true
	})
}
func (a *analyzer) suffix(n ast.Node) string {
	p := a.fset.PositionFor(n.Pos(), false)
	return filepath.Base(p.Filename) + fmt.Sprintf(":%d:%d", p.Line, p.Column)
}
func (a *analyzer) declare(sf *sourceFile, node ast.Node, body *ast.BlockStmt, values []ast.Expr, name, qualified, participant, params, returns string, obj *types.Func) *declaration {
	file, span := a.location(node)
	selector := qualified + params
	d := &declaration{callable: sequence.Callable{ID: sf.pkg.context + "::" + selector, Selector: selector, Qualified: qualified, Name: name, Participant: sf.pkg.context + "::" + participant, Returns: returns, File: file, Span: span}, node: node, body: body, values: values, object: obj, pkg: sf.pkg}
	a.declarations = append(a.declarations, d)
	return d
}
