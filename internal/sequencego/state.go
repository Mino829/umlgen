package sequencego

import (
	"context"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"sort"
	"strconv"
	"strings"

	"github.com/Mino829/umlgen/internal/activity"
	"github.com/Mino829/umlgen/internal/sequence"
	"github.com/Mino829/umlgen/internal/state"
)

// AnalyzeState collects semantic assignment candidates, never inferred runtime
// transitions. It consumes the loader's AST and type objects without constructing
// sequence bodies, executing the target, or loading additional source.
func AnalyzeState(ctx context.Context, sources []sequence.Source, typeSelector string, field string, maxNodes int) (result state.Analysis, err error) {
	a, err := loadAnalyzer(ctx, sources)
	result = state.Analysis{Language: a.program.Language, Backend: a.program.Backend, Files: a.program.Files}
	defer func() {
		a.sortDiagnostics()
		result.Diagnostics = a.program.Diagnostics
	}()
	if err != nil {
		return result, err
	}
	if maxNodes < 1 {
		return result, fmt.Errorf("activity max-nodes must be positive")
	}
	type candidate struct {
		target state.Target
		object *types.TypeName
		node   *ast.TypeSpec
	}
	var candidates []candidate
	var targets []state.Target
	for _, sf := range a.files {
		ast.Inspect(sf.ast, func(n ast.Node) bool {
			switch n.(type) {
			case *ast.FuncDecl, *ast.FuncLit, *ast.ValueSpec:
				if a.parserDamaged(sf, n) {
					return false
				}
			}
			spec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			if a.parserDamaged(sf, spec) {
				return false
			}
			obj, ok := sf.pkg.info.Defs[spec.Name].(*types.TypeName)
			if !ok {
				return false
			}
			file, span := a.location(spec)
			qualified := sf.pkg.path + "." + obj.Name()
			// Local type names need a declaration suffix to remain distinct.
			if obj.Parent() != sf.pkg.checked.Scope() {
				qualified += "@" + a.suffix(spec)
			}
			target := state.Target{ID: sf.pkg.context + "::" + qualified, Qualified: qualified, Name: obj.Name(), File: file, Span: span}
			targets = append(targets, target)
			candidates = append(candidates, candidate{target: target, object: obj, node: spec})
			return false
		})
	}
	selected, err := state.SelectTarget(targets, typeSelector)
	if err != nil {
		return result, err
	}
	result.Target = selected
	result.Target.Field = field
	var chosen candidate
	for _, c := range candidates {
		if c.target.ID == selected.ID {
			chosen = c
			break
		}
	}
	named, ok := chosen.object.Type().(*types.Named)
	if !ok || chosen.object.IsAlias() {
		return result, fmt.Errorf("%w: target %q must be a declared struct type, not an alias", state.ErrSelection, typeSelector)
	}
	structure, ok := named.Underlying().(*types.Struct)
	if !ok {
		return result, fmt.Errorf("%w: target %q is not a struct", state.ErrSelection, typeSelector)
	}
	if syntax, ok := chosen.node.Type.(*ast.StructType); ok {
		count := 0
		for _, declaration := range syntax.Fields.List {
			for _, name := range declaration.Names {
				if name.Name == field {
					count++
				}
			}
		}
		if field != "" && count > 1 {
			return result, fmt.Errorf("%w: %w: direct field %q", state.ErrSelection, state.ErrAmbiguous, field)
		}
	}
	var selectedField *types.Var
	for i := range structure.NumFields() {
		f := structure.Field(i)
		if field != "" && f.Name() == field {
			if selectedField != nil {
				return result, fmt.Errorf("%w: %w: direct field %q", state.ErrSelection, state.ErrAmbiguous, field)
			}
			selectedField = f.Origin()
		}
	}
	if selectedField == nil {
		return result, fmt.Errorf("%w: direct field %q not found in %s", state.ErrSelection, field, selected.ID)
	}
	basic, ok := selectedField.Type().Underlying().(*types.Basic)
	if !ok || basic.Info()&(types.IsBoolean|types.IsNumeric|types.IsString) == 0 {
		return result, fmt.Errorf("%w: field %q must have a scalar constant-compatible type", state.ErrSelection, field)
	}
	result.Target.Field, result.Target.FieldType = field, typeName(selectedField.Type())
	b := &stateBuilder{a: a, result: &result, field: selectedField, target: named.Origin(), basic: basic, declarations: map[ast.Node]*declaration{}, values: map[string]bool{}}
	for _, d := range a.declarations {
		b.declarations[d.node] = d
	}
	a.diagnostic("state-boundary", "Source candidates only: assignments do not establish a previous state, a configured transition, runtime receiver identity, alias effects, indirect calls, concurrency ordering, or implicit initialization.", chosen.node, false)
	for _, sf := range a.files {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		// Include unused selected-source constants as well as observed RHS values.
		ast.Inspect(sf.ast, func(n ast.Node) bool {
			if n == nil {
				return false
			}
			switch n.(type) {
			case *ast.FuncDecl, *ast.FuncLit, *ast.ValueSpec:
				if a.parserDamaged(sf, n) {
					return false
				}
			}
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			obj, ok := sf.pkg.info.Defs[id].(*types.Const)
			if !ok || !types.AssignableTo(obj.Type(), selectedField.Type()) {
				return true
			}
			key := b.valueKey(obj.Val())
			if key == "" {
				return true
			}
			qualified := sf.pkg.path + "." + obj.Name()
			if obj.Parent() != sf.pkg.checked.Scope() {
				qualified += "@" + a.suffix(id)
			}
			file, span := a.location(id)
			b.addValue(state.Value{ID: sf.pkg.context + "::" + qualified, Qualified: qualified, Name: obj.Name(), Key: key, Literal: obj.Val().ExactString(), File: file, Span: span})
			return true
		})
		ast.Walk(stateVisitor{b: b, sf: sf}, sf.ast)
	}
	for _, d := range a.declarations {
		if d.body == nil {
			continue
		}
		var updates []int
		for i := range result.Updates {
			if result.Updates[i].FunctionID == d.callable.ID {
				updates = append(updates, i)
			}
		}
		if len(updates) == 0 {
			continue
		}
		g := activity.Graph{Language: result.Language, Backend: result.Backend, Function: d.callable}
		g, graphErr := a.activityDeclaration(d, g, maxNodes)
		result.Functions = append(result.Functions, g)
		a.program.Diagnostics = append(a.program.Diagnostics, g.Diagnostics...)
		if graphErr != nil {
			return result, graphErr
		}
		for _, index := range updates {
			u := &result.Updates[index]
			correlationSpan := u.Span
			if binding := b.rangeBindings[index]; binding != nil {
				_, correlationSpan = a.location(binding)
			}
			width := int(^uint(0) >> 1)
			for _, n := range g.Nodes {
				// Initializer fields have no dedicated write action in the
				// activity model. A containing assignment/return can execute
				// while a short-circuit or constant operand is not evaluated.
				if u.Kind == "initializer" || n.Kind != "action" {
					continue
				}
				if n.File == u.File && n.Span.Start.Offset <= correlationSpan.Start.Offset && n.Span.End.Offset >= correlationSpan.End.Offset {
					w := n.Span.End.Offset - n.Span.Start.Offset
					if w < width {
						u.NodeIDs = []int{n.ID}
						width = w
					}
				}
			}
			if len(u.NodeIDs) == 0 {
				a.program.Diagnostics = append(a.program.Diagnostics, sequence.Diagnostic{Code: "state-cfg-unmodeled", Message: "Source write candidate has no modeled CFG node; unsupported control flow is not evidence of execution or absence.", File: u.File, Span: u.Span})
			}
		}
	}
	sort.Slice(result.Values, func(i, j int) bool { return result.Values[i].ID < result.Values[j].ID })
	sort.SliceStable(result.Updates, func(i, j int) bool {
		x, y := result.Updates[i], result.Updates[j]
		if x.File != y.File {
			return x.File < y.File
		}
		return x.Span.Start.Offset < y.Span.Start.Offset
	})
	return result, ctx.Err()
}

type stateBuilder struct {
	a             *analyzer
	result        *state.Analysis
	field         *types.Var
	target        *types.Named
	basic         *types.Basic
	declarations  map[ast.Node]*declaration
	values        map[string]bool
	rangeBindings map[int]ast.Node
}

// valueKey normalizes compiler constants to the selected field's scalar domain.
// No source expression is evaluated here; all values come from go/types.
func (b *stateBuilder) valueKey(v constant.Value) string {
	if v == nil || v.Kind() == constant.Unknown {
		return ""
	}
	kind := ""
	switch {
	case b.basic.Info()&types.IsString != 0:
		if v.Kind() != constant.String {
			return ""
		}
		kind = "string"
	case b.basic.Info()&types.IsBoolean != 0:
		if v.Kind() != constant.Bool {
			return ""
		}
		kind = "bool"
	case b.basic.Info()&types.IsInteger != 0:
		v = constant.ToInt(v)
		kind = "int"
		if v.Kind() == constant.Unknown {
			return ""
		}
		bits := 64
		switch b.basic.Kind() {
		case types.Int8, types.Uint8:
			bits = 8
		case types.Int16, types.Uint16:
			bits = 16
		case types.Int32, types.Uint32:
			bits = 32
		}
		if b.basic.Info()&types.IsUnsigned != 0 {
			n, ok := constant.Uint64Val(v)
			if !ok || (bits < 64 && n >= uint64(1)<<bits) {
				return ""
			}
		} else {
			n, ok := constant.Int64Val(v)
			if !ok || (bits < 64 && (n < -(int64(1)<<(bits-1)) || n >= int64(1)<<(bits-1))) {
				return ""
			}
		}
	case b.basic.Info()&types.IsFloat != 0:
		v = constant.ToFloat(v)
		kind = "float"
		if v.Kind() == constant.Unknown {
			return ""
		}
		if b.basic.Kind() == types.Float32 {
			n, _ := constant.Float32Val(v)
			v = constant.MakeFloat64(float64(n))
		} else {
			n, _ := constant.Float64Val(v)
			v = constant.MakeFloat64(n)
		}
	case b.basic.Info()&types.IsComplex != 0:
		v = constant.ToComplex(v)
		kind = "complex"
		if v.Kind() == constant.Unknown {
			return ""
		}
		real, imaginary := constant.Real(v), constant.Imag(v)
		if b.basic.Kind() == types.Complex64 {
			r, _ := constant.Float32Val(real)
			i, _ := constant.Float32Val(imaginary)
			real, imaginary = constant.MakeFloat64(float64(r)), constant.MakeFloat64(float64(i))
		} else {
			r, _ := constant.Float64Val(real)
			i, _ := constant.Float64Val(imaginary)
			real, imaginary = constant.MakeFloat64(r), constant.MakeFloat64(i)
		}
		if real.Kind() == constant.Unknown || imaginary.Kind() == constant.Unknown {
			return ""
		}
		v = constant.BinaryOp(real, token.ADD, constant.MakeImag(imaginary))
	}
	if kind == "" || v.Kind() == constant.Unknown {
		return ""
	}
	return "scalar:" + kind + ":" + v.ExactString()
}

func (b *stateBuilder) addValue(value state.Value) {
	if !b.values[value.ID] {
		b.values[value.ID] = true
		b.result.Values = append(b.result.Values, value)
	}
}

func (b *stateBuilder) text(n ast.Node) string {
	helper := activityBuilder{a: b.a}
	return helper.text(n)
}

func (b *stateBuilder) targetType(t types.Type) bool {
	if t == nil {
		return false
	}
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		t = types.Unalias(p.Elem())
	}
	n, ok := t.(*types.Named)
	return ok && n.Origin() == b.target
}

func (b *stateBuilder) selected(pkg *sourcePackage, expr ast.Expr) (*ast.SelectorExpr, bool) {
	for {
		p, ok := expr.(*ast.ParenExpr)
		if !ok {
			break
		}
		expr = p.X
	}
	s, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}
	selection := pkg.info.Selections[s]
	if selection == nil {
		return s, false
	}
	f, ok := selection.Obj().(*types.Var)
	if !ok || f.Origin() != b.field {
		return s, false
	}
	receiver := selection.Recv()
	for i, index := range selection.Index() {
		if i == len(selection.Index())-1 {
			return s, b.targetType(receiver)
		}
		receiver = types.Unalias(receiver)
		if pointer, ok := receiver.(*types.Pointer); ok {
			receiver = types.Unalias(pointer.Elem())
		}
		structure, ok := receiver.Underlying().(*types.Struct)
		if !ok || index >= structure.NumFields() {
			return s, false
		}
		receiver = structure.Field(index).Type()
	}
	return s, false
}

type stateVisitor struct {
	b      *stateBuilder
	sf     *sourceFile
	decl   *declaration
	guards []string
}

func (v stateVisitor) guarded(n ast.Node, guard string) {
	child := v
	child.guards = append(append([]string(nil), v.guards...), guard)
	ast.Walk(child, n)
}

func (v stateVisitor) update(kind string, n ast.Node, rhs ast.Expr, receiver string) {
	file, span := v.b.a.location(n)
	u := state.Update{Kind: kind, Receiver: receiver, File: file, Span: span, Guards: append([]string(nil), v.guards...)}
	if v.decl != nil && v.decl.body != nil {
		u.FunctionID = v.decl.callable.ID
	}
	if rhs != nil {
		u.Expression = v.b.text(rhs)
		if kind == "assignment" || kind == "initializer" {
			if tv, ok := v.sf.pkg.info.Types[rhs]; ok && tv.Value != nil && types.AssignableTo(tv.Type, v.b.field.Type()) {
				u.ValueKey = v.b.valueKey(tv.Value)
				var boundConstant *types.Const
				switch expr := rhs.(type) {
				case *ast.Ident:
					boundConstant, _ = v.sf.pkg.info.Uses[expr].(*types.Const)
				case *ast.SelectorExpr:
					boundConstant, _ = v.sf.pkg.info.Uses[expr.Sel].(*types.Const)
				}
				if u.ValueKey != "" && (boundConstant == nil || boundConstant.Pkg() == nil) {
					valueFile, valueSpan := v.b.a.location(rhs)
					v.b.addValue(state.Value{ID: "literal::" + valueFile + "@" + strconv.Itoa(valueSpan.Start.Offset), Qualified: u.Expression, Name: u.Expression, Key: u.ValueKey, Literal: u.Expression, File: valueFile, Span: valueSpan})
				}
			}
		}
	}
	v.b.result.Updates = append(v.b.result.Updates, u)
	if u.FunctionID == "" {
		v.b.a.diagnostic("state-initialization", "Explicit initializer candidate is outside a modeled function; implicit initialization order and an initial state are not inferred.", n, false)
	}
	v.b.a.diagnostic("state-receiver-identity", "Write targets the selected field declaration; receiver aliasing and identity with any other candidate's runtime instance are not proven.", n, false)
}

func (v stateVisitor) Visit(n ast.Node) ast.Visitor {
	if n == nil || v.b.a.ctx.Err() != nil {
		return nil
	}
	switch n.(type) {
	case *ast.FuncDecl, *ast.FuncLit, *ast.ValueSpec, *ast.TypeSpec:
		if v.b.a.parserDamaged(v.sf, n) {
			return nil
		}
	}
	switch s := n.(type) {
	case *ast.FuncDecl, *ast.FuncLit:
		v.decl = v.b.declarations[n]
		v.guards = nil
		if v.decl == nil {
			return nil
		}
	case *ast.ValueSpec:
		if len(s.Values) == 0 {
			for _, name := range s.Names {
				if obj := v.sf.pkg.info.Defs[name]; obj != nil && v.b.targetType(obj.Type()) {
					v.b.a.diagnostic("state-implicit-initialization", "Variable has no explicit initializer; implicit zero initialization is not an initial state or a source assignment candidate.", s, false)
				}
			}
		}
	case *ast.IfStmt:
		if s.Init != nil {
			ast.Walk(v, s.Init)
		}
		ast.Walk(v, s.Cond)
		v.guarded(s.Body, "if ("+v.b.text(s.Cond)+")")
		if s.Else != nil {
			v.guarded(s.Else, "else of ("+v.b.text(s.Cond)+")")
		}
		return nil
	case *ast.ForStmt:
		if s.Init != nil {
			ast.Walk(v, s.Init)
		}
		if s.Cond != nil {
			ast.Walk(v, s.Cond)
		}
		guard := "for"
		if s.Cond != nil {
			guard += " (" + v.b.text(s.Cond) + ")"
		}
		v.guarded(s.Body, guard)
		if s.Post != nil {
			v.guarded(s.Post, guard)
		}
		return nil
	case *ast.RangeStmt:
		ast.Walk(v, s.X)
		for _, lhs := range []ast.Expr{s.Key, s.Value} {
			if lhs == nil {
				continue
			}
			if sel, ok := v.b.selected(v.sf.pkg, lhs); ok {
				v.update("range", lhs, s.X, v.b.text(sel.X))
				if v.b.rangeBindings == nil {
					v.b.rangeBindings = map[int]ast.Node{}
				}
				v.b.rangeBindings[len(v.b.result.Updates)-1] = s.Key
			}
			v.guarded(lhs, "range "+v.b.text(s.X))
		}
		v.guarded(s.Body, "range "+v.b.text(s.X))
		return nil
	case *ast.SwitchStmt:
		if s.Init != nil {
			ast.Walk(v, s.Init)
		}
		if s.Tag != nil {
			ast.Walk(v, s.Tag)
		}
		guard := "switch"
		if s.Tag != nil {
			guard += " " + v.b.text(s.Tag)
		}
		v.guarded(s.Body, guard)
		return nil
	case *ast.CaseClause:
		for _, e := range s.List {
			ast.Walk(v, e)
		}
		guard := "default"
		if len(s.List) > 0 {
			var parts []string
			for _, e := range s.List {
				parts = append(parts, v.b.text(e))
			}
			guard = "case " + strings.Join(parts, ", ")
		}
		for _, stmt := range s.Body {
			v.guarded(stmt, guard)
		}
		return nil
	case *ast.AssignStmt:
		for i, lhs := range s.Lhs {
			if sel, ok := v.b.selected(v.sf.pkg, lhs); ok {
				var rhs ast.Expr
				if len(s.Rhs) == len(s.Lhs) {
					rhs = s.Rhs[i]
				} else if len(s.Rhs) == 1 {
					rhs = s.Rhs[0]
				}
				kind := "assignment"
				if s.Tok != token.ASSIGN && s.Tok != token.DEFINE {
					kind = "compound"
				}
				v.update(kind, s, rhs, v.b.text(sel.X))
			} else if sel != nil && sel.Sel.Name == v.b.field.Name() && v.sf.pkg.info.Selections[sel] == nil {
				v.b.a.diagnostic("state-unresolved-write", "Cannot bind this same-spelled field write; no selected-field update is proven.", s, false)
			} else if v.b.targetType(v.sf.pkg.info.TypeOf(lhs)) {
				v.b.a.diagnostic("state-whole-value-write", "Whole struct/pointer assignment may replace or alias the selected field; field-level effect is not resolved.", s, false)
			}
		}
	case *ast.IncDecStmt:
		if sel, ok := v.b.selected(v.sf.pkg, s.X); ok {
			kind := "increment"
			if s.Tok == token.DEC {
				kind = "decrement"
			}
			v.update(kind, s, nil, v.b.text(sel.X))
		}
	case *ast.UnaryExpr:
		if s.Op == token.AND {
			if _, ok := v.b.selected(v.sf.pkg, s.X); ok {
				v.b.a.diagnostic("state-field-alias", "Address of selected field escapes direct field binding; writes through the alias are not resolved.", s, false)
			}
		}
	case *ast.CallExpr:
		if id, ok := s.Fun.(*ast.Ident); ok {
			if builtin, ok := v.sf.pkg.info.Uses[id].(*types.Builtin); ok && builtin.Name() == "new" && v.b.targetType(v.sf.pkg.info.TypeOf(s)) {
				v.b.a.diagnostic("state-implicit-initialization", "Allocation implicitly zero-initializes the selected field; no initial state or explicit field write is inferred.", s, false)
			}
		}
		if sel, ok := s.Fun.(*ast.SelectorExpr); ok && v.b.targetType(v.sf.pkg.info.TypeOf(sel.X)) {
			v.b.a.diagnostic("state-call-alias", "Method receiver has selected struct type; interprocedural mutation and runtime receiver identity are not resolved.", s, false)
		}
		for _, arg := range s.Args {
			if v.b.targetType(v.sf.pkg.info.TypeOf(arg)) {
				v.b.a.diagnostic("state-call-alias", "Selected struct or pointer is passed to a call; indirect mutation, escape, and runtime receiver identity are not resolved.", arg, false)
			}
		}
	case *ast.GoStmt:
		v.b.a.diagnostic("state-concurrency", "Goroutine execution and ordering of state write candidates are not modeled.", s, false)
	case *ast.DeferStmt:
		v.b.a.diagnostic("state-deferred", "Deferred execution order and recover effects are not modeled for state candidates.", s, false)
	case *ast.CompositeLit:
		t := v.sf.pkg.info.TypeOf(s)
		if !v.b.targetType(t) {
			break
		}
		structure, ok := t.Underlying().(*types.Struct)
		if !ok {
			break
		}
		found := false
		for i, element := range s.Elts {
			expr := element
			var f *types.Var
			if pair, ok := element.(*ast.KeyValueExpr); ok {
				expr = pair.Value
				if id, ok := pair.Key.(*ast.Ident); ok {
					f, _ = v.sf.pkg.info.Uses[id].(*types.Var)
				}
			} else if i < structure.NumFields() {
				f = structure.Field(i)
			}
			if f != nil && f.Origin() == v.b.field {
				v.update("initializer", element, expr, v.b.text(s.Type))
				found = true
			}
		}
		if !found {
			v.b.a.diagnostic("state-implicit-initialization", "Composite literal omits the selected field; its implicit zero value is not an explicit initial state or write candidate.", s, false)
		}
	}
	return v
}
