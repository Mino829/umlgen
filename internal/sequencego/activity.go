package sequencego

import (
	"context"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"github.com/Mino829/umlgen/internal/activity"
	"github.com/Mino829/umlgen/internal/sequence"
)

// AnalyzeActivity projects one selected source function without constructing
// sequence events, expanding callees, or loading any unselected dependencies.
func AnalyzeActivity(ctx context.Context, sources []sequence.Source, selector string, maxNodes int) (activity.Graph, error) {
	a, err := loadAnalyzer(ctx, sources)
	a.sortDiagnostics()
	g := activity.Graph{Language: a.program.Language, Backend: a.program.Backend, Files: a.program.Files, Diagnostics: a.program.Diagnostics}
	g.Diagnostics = append(g.Diagnostics, sequence.Diagnostic{Code: "activity-boundary", Message: "Intraprocedural syntactic control flow only: direct calls are actions; callees, implicit exceptional transfers and implicit initialization are not expanded."})
	if err != nil {
		return g, err
	}
	for _, d := range a.declarations {
		// Package initializers are not function entries.
		if d.body != nil {
			a.program.Callables = append(a.program.Callables, d.callable)
		}
	}
	selected, err := sequence.Select(a.program, selector)
	if err != nil {
		return g, fmt.Errorf("%w: %w", activity.ErrSelection, err)
	}
	g.Function = selected
	if maxNodes < 1 {
		return g, fmt.Errorf("activity max-nodes must be positive")
	}
	var decl *declaration
	for _, d := range a.declarations {
		if d.callable.ID == selected.ID {
			decl = d
			break
		}
	}
	return a.activityDeclaration(decl, g, maxNodes)
}

// activityDeclaration projects an already loaded declaration; state evidence and
// activity analysis share both semantic bindings and CFG construction.
func (a *analyzer) activityDeclaration(decl *declaration, g activity.Graph, maxNodes int) (activity.Graph, error) {
	b := activityBuilder{a: a, pkg: decl.pkg, graph: &g, maxNodes: maxNodes, labels: map[string]int{}}
	g.Entry = b.node("entry", decl.callable.Name, decl.node)
	g.Exit = b.node("exit", "exit", decl.node)
	// Go labels have function scope, including forward gotos. Nested literal
	// bodies belong to other functions and must not contribute labels or paths.
	ast.Inspect(decl.body, func(n ast.Node) bool {
		if b.err != nil {
			return false
		}
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		if label, ok := n.(*ast.LabeledStmt); ok {
			if _, exists := b.labels[label.Label.Name]; !exists {
				b.labels[label.Label.Name] = b.node("merge", "label "+label.Label.Name, label.Label)
			}
		}
		return true
	})
	out := b.statements(decl.body.List, []activityPending{{from: g.Entry}}, activityScope{})
	b.connect(out, g.Exit)
	if b.err != nil {
		// Never return a truncated drawable graph on a hard construction limit.
		g.Nodes, g.Edges = nil, nil
		g.Entry, g.Exit = 0, 0
		return g, b.err
	}
	if err := g.Validate(maxNodes); err != nil {
		return g, err
	}
	return g, a.ctx.Err()
}

type activityPending struct {
	from  int
	kind  string
	label string
}

type activityTarget struct {
	label      string
	breakTo    int
	continueTo int
}

type activityScope struct {
	targets       []activityTarget
	fallthroughTo int
}

func (s activityScope) push(t activityTarget) activityScope {
	// Targets are consumed synchronously within this lexical child scope;
	// appending never changes the parent's visible stack length.
	return activityScope{targets: append(s.targets, t), fallthroughTo: s.fallthroughTo}
}

type activityBuilder struct {
	a        *analyzer
	pkg      *sourcePackage
	graph    *activity.Graph
	maxNodes int
	labels   map[string]int
	err      error
}

func (b *activityBuilder) node(kind, label string, n ast.Node) int {
	if b.err != nil {
		return 0
	}
	if err := b.a.ctx.Err(); err != nil {
		b.err = err
		return 0
	}
	if len(b.graph.Nodes) >= b.maxNodes {
		b.err = fmt.Errorf("%w: cap %d; increase --max-nodes", activity.ErrLimit, b.maxNodes)
		return 0
	}
	file, span := b.a.location(n)
	id := len(b.graph.Nodes) + 1
	b.graph.Nodes = append(b.graph.Nodes, activity.Node{ID: id, Kind: kind, Label: label, File: file, Span: span})
	return id
}

func (b *activityBuilder) edge(from, to int, kind, label string) {
	if b.err != nil || from == 0 || to == 0 {
		return
	}
	if kind == "" {
		kind = "normal"
	}
	b.graph.Edges = append(b.graph.Edges, activity.Edge{From: from, To: to, Kind: kind, Label: label})
}

func (b *activityBuilder) connect(in []activityPending, to int) {
	for _, p := range in {
		b.edge(p.from, to, p.kind, p.label)
	}
}

func (b *activityBuilder) text(n ast.Node) string {
	file, span := b.a.location(n)
	source := b.a.sourceByPath[file]
	if span.Start.Offset < 0 || span.End.Offset > len(source.Content) || span.End.Offset < span.Start.Offset {
		return "<?>"
	}
	return strings.TrimSpace(string(source.Content[span.Start.Offset:span.End.Offset]))
}

func (b *activityBuilder) action(n ast.Node, in []activityPending) []activityPending {
	id := b.node("action", b.text(n), n)
	b.connect(in, id)
	return []activityPending{{from: id}}
}

func (b *activityBuilder) unsupported(n ast.Node, in []activityPending, reason string) []activityPending {
	id := b.node("unsupported", reason+": "+b.text(n), n)
	b.connect(in, id)
	file, span := b.a.location(n)
	b.graph.Diagnostics = append(b.graph.Diagnostics, sequence.Diagnostic{Code: "activity-unsupported", Message: reason, File: file, Span: span})
	return nil
}

func (b *activityBuilder) statements(list []ast.Stmt, in []activityPending, scope activityScope) []activityPending {
	for _, stmt := range list {
		if b.err != nil {
			break
		}
		in = b.statement(stmt, in, scope, "")
	}
	return in
}

func (b *activityBuilder) statement(stmt ast.Stmt, in []activityPending, scope activityScope, label string) []activityPending {
	if stmt == nil || b.err != nil {
		return in
	}
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		return b.statements(s.List, in, scope)
	case *ast.EmptyStmt:
		return in
	case *ast.LabeledStmt:
		id := b.labels[s.Label.Name]
		b.connect(in, id)
		return b.statement(s.Stmt, []activityPending{{from: id}}, scope, s.Label.Name)
	case *ast.IfStmt:
		in = b.statement(s.Init, in, scope, "")
		yes, no := b.condition(s.Cond, in)
		yes = b.statements(s.Body.List, yes, scope)
		if s.Else != nil {
			no = b.statement(s.Else, no, scope, "")
		}
		return append(yes, no...)
	case *ast.ForStmt:
		return b.forStatement(s, in, scope, label)
	case *ast.RangeStmt:
		return b.rangeStatement(s, in, scope, label)
	case *ast.SwitchStmt:
		return b.switchStatement(s, in, scope, label)
	case *ast.ReturnStmt:
		for _, expr := range s.Results {
			in = b.expression(expr, in)
		}
		id := b.node("return", b.text(s), s)
		b.connect(in, id)
		b.edge(id, b.graph.Exit, "return", "")
		return nil
	case *ast.BranchStmt:
		return b.branch(s, in, scope)
	case *ast.ExprStmt:
		if _, ok := s.X.(*ast.CallExpr); ok {
			return b.expression(s.X, in)
		}
		in = b.expression(s.X, in)
		return b.action(s, in)
	case *ast.AssignStmt:
		for _, expr := range s.Lhs {
			in = b.expression(expr, in)
		}
		for _, expr := range s.Rhs {
			in = b.expression(expr, in)
		}
		return b.action(s, in)
	case *ast.IncDecStmt:
		in = b.expression(s.X, in)
		return b.action(s, in)
	case *ast.DeclStmt:
		if d, ok := s.Decl.(*ast.GenDecl); ok {
			for _, spec := range d.Specs {
				if value, ok := spec.(*ast.ValueSpec); ok {
					for _, expr := range value.Values {
						in = b.expression(expr, in)
					}
				}
			}
		}
		return b.action(s, in)
	case *ast.DeferStmt:
		return b.unsupported(s, in, "defer/recover transfer is not modeled")
	case *ast.GoStmt:
		return b.unsupported(s, in, "goroutine execution is not modeled")
	case *ast.SelectStmt:
		return b.unsupported(s, in, "select communication is not modeled")
	case *ast.SendStmt:
		return b.unsupported(s, in, "channel communication is not modeled")
	case *ast.TypeSwitchStmt:
		var guard ast.Expr
		switch assign := s.Assign.(type) {
		case *ast.ExprStmt:
			guard = assign.X
		case *ast.AssignStmt:
			if len(assign.Rhs) == 1 {
				guard = assign.Rhs[0]
			}
		}
		if assertion, ok := guard.(*ast.TypeAssertExpr); ok && assertion.Type == nil {
			// The guard's calls execute once; case types are not executed.
			return b.switchStatement(&ast.SwitchStmt{Switch: s.Switch, Init: s.Init, Tag: assertion, Body: s.Body}, in, scope, label)
		}
		return b.unsupported(s, in, "invalid type-switch guard")
	default:
		return b.unsupported(stmt, in, "unmodeled Go statement")
	}
}

func (b *activityBuilder) forStatement(s *ast.ForStmt, in []activityPending, scope activityScope, label string) []activityPending {
	in = b.statement(s.Init, in, scope, "")
	head := b.node("merge", "for condition", s)
	after := b.node("merge", "after for", s)
	post := head
	if s.Post != nil {
		post = b.node("merge", "for update", s.Post)
	}
	b.connect(in, head)
	body := []activityPending{{from: head}}
	if s.Cond != nil {
		var no []activityPending
		body, no = b.condition(s.Cond, body)
		b.connect(no, after)
	}
	inner := scope.push(activityTarget{label: label, breakTo: after, continueTo: post})
	out := b.statements(s.Body.List, body, inner)
	if s.Post != nil {
		b.connect(out, post)
		out = b.statement(s.Post, []activityPending{{from: post}}, scope, "")
	}
	for _, p := range out {
		b.edge(p.from, head, "back", "")
	}
	return []activityPending{{from: after}}
}

func (b *activityBuilder) rangeStatement(s *ast.RangeStmt, in []activityPending, scope activityScope, label string) []activityPending {
	// Channel range and iterator functions have implicit communication/callee
	// execution that this intraprocedural projection cannot honestly expand.
	if activityRangeHasEffects(b.pkg.info.TypeOf(s.X)) {
		return b.unsupported(s, in, "channel/iterator range is not modeled")
	}
	in = b.expression(s.X, in)
	head := b.node("decision", "range "+b.text(s.X), s.X)
	after := b.node("merge", "after range", s)
	b.connect(in, head)
	b.edge(head, after, "false", "exhausted / zero iterations")
	inner := scope.push(activityTarget{label: label, breakTo: after, continueTo: head})
	body := []activityPending{{from: head, kind: "true", label: "next item"}}
	if s.Key != nil {
		body = b.expression(s.Key, body)
		body = b.expression(s.Value, body)
		bind := "range bind " + b.text(s.Key)
		if s.Value != nil {
			bind += ", " + b.text(s.Value)
		}
		id := b.node("action", bind, s.Key)
		b.connect(body, id)
		body = []activityPending{{from: id}}
	}
	out := b.statements(s.Body.List, body, inner)
	for _, p := range out {
		b.edge(p.from, head, "back", "")
	}
	return []activityPending{{from: after}}
}

func activityRangeHasEffects(t types.Type) bool {
	if t == nil {
		return false
	}
	switch underlying := t.Underlying().(type) {
	case *types.Chan, *types.Signature:
		return true
	case *types.Interface:
		// Type parameters expose their constraint as an interface, not the
		// channel/iterator core type of each admissible instantiation.
		return activityConstraintHasEffects(underlying, make(map[types.Type]bool))
	}
	return false
}

func activityConstraintHasEffects(t types.Type, seen map[types.Type]bool) bool {
	if seen[t] {
		return false
	}
	seen[t] = true
	switch underlying := t.Underlying().(type) {
	case *types.Chan, *types.Signature:
		return true
	case *types.Interface:
		for i := 0; i < underlying.NumEmbeddeds(); i++ {
			if activityConstraintHasEffects(underlying.EmbeddedType(i), seen) {
				return true
			}
		}
	case *types.Union:
		for i := 0; i < underlying.Len(); i++ {
			if activityConstraintHasEffects(underlying.Term(i).Type(), seen) {
				return true
			}
		}
	}
	return false
}

func (b *activityBuilder) branch(s *ast.BranchStmt, in []activityPending, scope activityScope) []activityPending {
	// The type checker catches invalid jumps into scopes and other illegal
	// transfers. Do not turn their recovered AST into plausible control flow.
	file, span := b.a.location(s)
	for _, d := range b.a.program.Diagnostics {
		if d.Code == "type-error" && d.File == file && d.Span.Start.Offset >= span.Start.Offset && d.Span.Start.Offset < span.End.Offset {
			return b.unsupported(s, in, "invalid branch transfer: "+d.Message)
		}
	}
	name := ""
	if s.Label != nil {
		name = s.Label.Name
	}
	target := 0
	switch s.Tok {
	case token.GOTO:
		target = b.labels[name]
	case token.FALLTHROUGH:
		target = scope.fallthroughTo
	case token.BREAK, token.CONTINUE:
		for i := len(scope.targets) - 1; i >= 0; i-- {
			t := scope.targets[i]
			if name != "" && t.label != name {
				continue
			}
			if s.Tok == token.CONTINUE {
				target = t.continueTo
				if target == 0 && name == "" {
					continue
				}
			} else {
				target = t.breakTo
			}
			break
		}
	}
	if target == 0 {
		return b.unsupported(s, in, "unresolved branch target")
	}
	id := b.node("action", b.text(s), s)
	b.connect(in, id)
	b.edge(id, target, s.Tok.String(), name)
	return nil
}

func (b *activityBuilder) switchStatement(s *ast.SwitchStmt, in []activityPending, scope activityScope, label string) []activityPending {
	in = b.statement(s.Init, in, scope, "")
	if s.Tag != nil {
		in = b.expression(s.Tag, in)
	}
	after := b.node("merge", "after switch", s)
	capacity := min(len(s.Body.List), b.maxNodes)
	clauses := make([]*ast.CaseClause, 0, capacity)
	entries := make([]int, 0, capacity)
	defaultTo := after
	for _, stmt := range s.Body.List {
		if b.err != nil {
			return nil
		}
		clause, ok := stmt.(*ast.CaseClause)
		if !ok {
			return b.unsupported(stmt, in, "unmodeled switch clause")
		}
		clauses = append(clauses, clause)
		text := "default"
		if clause.List != nil {
			parts := make([]string, len(clause.List))
			for i, expr := range clause.List {
				parts[i] = b.text(expr)
			}
			text = "case " + strings.Join(parts, ", ")
		}
		id := b.node("merge", text, clause)
		entries = append(entries, id)
		if clause.List == nil {
			defaultTo = id
		}
	}
	// Case expressions are tested in source order; default runs only after all
	// non-default alternatives fail, even when textually before another case.
	for i, clause := range clauses {
		for _, expr := range clause.List {
			if b.err != nil {
				return nil
			}
			if s.Tag == nil {
				yes, no := b.condition(expr, in)
				for _, p := range yes {
					b.edge(p.from, entries[i], p.kind, "case "+b.text(expr))
				}
				in = no
			} else {
				in = b.expression(expr, in)
				test := b.text(s.Tag) + " == " + b.text(expr)
				if assertion, ok := s.Tag.(*ast.TypeAssertExpr); ok && assertion.Type == nil {
					test = "type(" + b.text(assertion.X) + ") is " + b.text(expr)
				}
				id := b.node("decision", test, expr)
				b.connect(in, id)
				b.edge(id, entries[i], "case", b.text(expr))
				in = []activityPending{{from: id, kind: "false"}}
			}
		}
	}
	for _, p := range in {
		b.edge(p.from, defaultTo, "default", "no matching case")
	}
	for i, clause := range clauses {
		inner := scope.push(activityTarget{label: label, breakTo: after})
		inner.fallthroughTo = 0
		if i+1 < len(entries) {
			inner.fallthroughTo = entries[i+1]
		}
		out := b.statements(clause.Body, []activityPending{{from: entries[i]}}, inner)
		b.connect(out, after)
	}
	return []activityPending{{from: after}}
}

// condition returns separate true/false frontiers. RHS nodes only have incoming
// edges from the LHS outcome that actually evaluates them.
func (b *activityBuilder) condition(expr ast.Expr, in []activityPending) ([]activityPending, []activityPending) {
	if b.err != nil {
		return nil, nil
	}
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return b.condition(e.X, in)
	case *ast.BinaryExpr:
		if e.Op == token.LAND {
			yes, no := b.condition(e.X, in)
			rightYes, rightNo := b.condition(e.Y, yes)
			return rightYes, append(no, rightNo...)
		}
		if e.Op == token.LOR {
			yes, no := b.condition(e.X, in)
			rightYes, rightNo := b.condition(e.Y, no)
			return append(yes, rightYes...), rightNo
		}
	case *ast.UnaryExpr:
		if e.Op == token.NOT {
			yes, no := b.condition(e.X, in)
			return no, yes
		}
	}
	in = b.expression(expr, in)
	id := b.node("decision", b.text(expr), expr)
	b.connect(in, id)
	if value := b.pkg.info.Types[expr].Value; value != nil && value.Kind() == constant.Bool {
		if constant.BoolVal(value) {
			return []activityPending{{from: id, kind: "true"}}, nil
		}
		return nil, []activityPending{{from: id, kind: "false"}}
	}
	return []activityPending{{from: id, kind: "true"}}, []activityPending{{from: id, kind: "false"}}
}

func (b *activityBuilder) builtin(call *ast.CallExpr) string {
	fun := call.Fun
	for {
		paren, ok := fun.(*ast.ParenExpr)
		if !ok {
			break
		}
		fun = paren.X
	}
	if id, ok := fun.(*ast.Ident); ok {
		if obj, ok := b.pkg.info.Uses[id].(*types.Builtin); ok {
			return obj.Name()
		}
	}
	return ""
}

// expression emits evaluation effects, not literal/identifier reads. In
// particular it never descends into a function literal's unexecuted body.
func (b *activityBuilder) expression(expr ast.Expr, in []activityPending) []activityPending {
	if expr == nil || b.err != nil {
		return in
	}
	switch e := expr.(type) {
	case *ast.FuncLit, *ast.Ident, *ast.BasicLit:
		return in
	case *ast.ParenExpr:
		return b.expression(e.X, in)
	case *ast.BinaryExpr:
		if e.Op == token.LAND || e.Op == token.LOR {
			yes, no := b.condition(e, in)
			id := b.node("merge", "boolean result", e)
			b.connect(append(yes, no...), id)
			return []activityPending{{from: id}}
		}
		in = b.expression(e.X, in)
		return b.expression(e.Y, in)
	case *ast.UnaryExpr:
		if e.Op == token.ARROW {
			return b.unsupported(e, in, "channel receive is not modeled")
		}
		return b.expression(e.X, in)
	case *ast.CallExpr:
		// go/types identifies constant layout/length operations whose
		// arguments are not evaluated (notably unsafe.Sizeof/Alignof/Offsetof).
		if b.pkg.info.Types[e].Value != nil {
			return in
		}
		in = b.expression(e.Fun, in)
		for _, arg := range e.Args {
			in = b.expression(arg, in)
		}
		switch b.builtin(e) {
		case "panic":
			id := b.node("throw", b.text(e), e)
			b.connect(in, id)
			b.edge(id, b.graph.Exit, "panic", "")
			return nil
		case "recover":
			return b.unsupported(e, in, "defer/recover transfer is not modeled")
		}
		return b.action(e, in)
	case *ast.SelectorExpr:
		return b.expression(e.X, in)
	case *ast.IndexExpr:
		in = b.expression(e.X, in)
		return b.expression(e.Index, in)
	case *ast.IndexListExpr:
		return b.expression(e.X, in)
	case *ast.SliceExpr:
		in = b.expression(e.X, in)
		in = b.expression(e.Low, in)
		in = b.expression(e.High, in)
		return b.expression(e.Max, in)
	case *ast.StarExpr:
		return b.expression(e.X, in)
	case *ast.TypeAssertExpr:
		return b.expression(e.X, in)
	case *ast.CompositeLit:
		for _, elt := range e.Elts {
			in = b.expression(elt, in)
		}
		return in
	case *ast.KeyValueExpr:
		in = b.expression(e.Key, in)
		return b.expression(e.Value, in)
	case *ast.ArrayType, *ast.MapType, *ast.StructType, *ast.InterfaceType, *ast.FuncType, *ast.ChanType, *ast.Ellipsis:
		return in
	default:
		return b.unsupported(expr, in, "unmodeled Go expression")
	}
}
