package sequencego

import (
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"github.com/Mino829/umlgen/internal/sequence"
)

type builder struct {
	a   *analyzer
	pkg *sourcePackage
}

func (b builder) event(kind sequence.Kind, n ast.Node, label string) sequence.Event {
	file, span := b.a.location(n)
	return sequence.Event{Kind: kind, Label: label, File: file, Span: span}
}
func (b builder) text(n ast.Node) string {
	file, span := b.a.location(n)
	if source, ok := b.a.sourceByPath[file]; ok {
		start, end := span.Start.Offset, span.End.Offset
		if start >= 0 && end >= start && end <= len(source.Content) {
			return string(source.Content[start:end])
		}
	}
	return "<unavailable source>"
}
func (b builder) unsupported(n ast.Node, message string) sequence.Event {
	b.a.diagnostic("unsupported", message, n, false)
	return b.event(sequence.Unsupported, n, message)
}
func unparen(expr ast.Expr) ast.Expr {
	for {
		p, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = p.X
	}
}
func (b builder) expr(expr ast.Expr) []sequence.Event {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.Ident, *ast.BasicLit, *ast.FuncLit:
		return nil
	case *ast.ParenExpr:
		return b.expr(e.X)
	case *ast.CallExpr:
		return b.call(e, sequence.Call)
	case *ast.SelectorExpr:
		return b.expr(e.X)
	case *ast.IndexExpr:
		return append(b.expr(e.X), b.expr(e.Index)...)
	case *ast.IndexListExpr:
		events := b.expr(e.X)
		for _, i := range e.Indices {
			events = append(events, b.expr(i)...)
		}
		return events
	case *ast.SliceExpr:
		events := b.expr(e.X)
		events = append(events, b.expr(e.Low)...)
		events = append(events, b.expr(e.High)...)
		return append(events, b.expr(e.Max)...)
	case *ast.StarExpr:
		return b.expr(e.X)
	case *ast.UnaryExpr:
		events := b.expr(e.X)
		if e.Op == token.ARROW {
			events = append(events, b.event(sequence.Receive, e, "receive "+b.text(e.X)+" (communication peer unknown)"))
		}
		return events
	case *ast.BinaryExpr:
		events := b.expr(e.X)
		if e.Op == token.LAND || e.Op == token.LOR {
			guard := b.text(e.X)
			if e.Op == token.LOR {
				guard = "not (" + guard + ")"
			}
			ev := b.event(sequence.Opt, e.Y, guard)
			ev.Children = b.expr(e.Y)
			return append(events, ev)
		}
		return append(events, b.expr(e.Y)...)
	case *ast.CompositeLit:
		events := []sequence.Event{}
		for _, elt := range e.Elts {
			events = append(events, b.expr(elt)...)
		}
		return events
	case *ast.KeyValueExpr:
		return append(b.expr(e.Key), b.expr(e.Value)...)
	case *ast.TypeAssertExpr:
		return b.expr(e.X)
	case *ast.ArrayType, *ast.MapType, *ast.ChanType, *ast.FuncType, *ast.StructType, *ast.InterfaceType, *ast.Ellipsis:
		return nil
	case *ast.BadExpr:
		return []sequence.Event{b.unsupported(e, "parser could not recover expression")}
	default:
		return []sequence.Event{b.unsupported(e, "unsupported Go expression")}
	}
}
func (b builder) call(call *ast.CallExpr, kind sequence.Kind) []sequence.Event {
	// go/types marks constant unsafe layout operations. Their operands are not
	// evaluated, even when the source operand contains a call or receive.
	if value := b.pkg.info.Types[call].Value; value != nil {
		if selector, ok := unparen(call.Fun).(*ast.SelectorExpr); ok {
			if builtin, ok := b.pkg.info.Uses[selector.Sel].(*types.Builtin); ok && builtin.Pkg() != nil && builtin.Pkg().Path() == "unsafe" {
				ev := b.event(sequence.Note, call, builtin.Name()+" is constant; operand is not evaluated")
				return []sequence.Event{ev}
			}
		}
	}
	events := b.expr(call.Fun)
	for _, arg := range call.Args {
		events = append(events, b.expr(arg)...)
	}
	if tv, ok := b.pkg.info.Types[call.Fun]; ok && tv.IsType() {
		return events
	}
	ev := b.event(kind, call, b.text(call.Fun))
	base := unparen(call.Fun)
	for {
		switch e := base.(type) {
		case *ast.IndexExpr:
			base = unparen(e.X)
			continue
		case *ast.IndexListExpr:
			base = unparen(e.X)
			continue
		}
		break
	}
	var obj types.Object
	var selection *types.Selection
	switch fun := base.(type) {
	case *ast.Ident:
		obj = b.pkg.info.Uses[fun]
	case *ast.SelectorExpr:
		selection = b.pkg.info.Selections[fun]
		if selection != nil {
			obj = selection.Obj()
		} else {
			obj = b.pkg.info.Uses[fun.Sel]
		}
	case *ast.FuncLit:
		if d := b.a.literals[fun]; d != nil {
			ev.Dispatch = "direct"
			ev.Target = d.callable.ID
			ev.Label = d.callable.Selector
		}
	}
	if ev.Dispatch == "" {
		switch object := obj.(type) {
		case *types.Builtin:
			ev.Dispatch = "builtin"
			ev.Label = object.Name()
		case *types.Func:
			dynamic := false
			if selection != nil {
				_, dynamic = selection.Recv().Underlying().(*types.Interface)
			}
			if dynamic {
				ev.Dispatch = "interface"
				ev.Label = b.text(call.Fun)
				ev.Candidates = b.interfaceCandidates(selection, object)
			} else if d := b.a.functions[object.Origin()]; d != nil {
				ev.Dispatch = "direct"
				ev.Target = d.callable.ID
				ev.Label = d.callable.Selector
			} else {
				ev.Dispatch = "external"
				ev.Label = object.FullName()
			}
		default:
			typ := b.pkg.info.TypeOf(call.Fun)
			if typ != nil {
				if sig, ok := typ.Underlying().(*types.Signature); ok {
					ev.Dispatch = "function-value"
					ev.Label = b.text(call.Fun)
					ev.Candidates = b.functionCandidates(sig)
				}
			}
			if _, ok := typ.(*types.TypeParam); ok && b.pkg.info.TypeOf(call) != nil {
				ev.Dispatch = "function-value"
				ev.Label = b.text(call.Fun)
			}
			if ev.Dispatch == "" {
				ev.Dispatch = "unresolved"
				ev.Label = b.text(call.Fun) + " (unresolved binding)"
				b.a.diagnostic("unresolved-call", "Call has no reliable semantic binding", call, true)
			}
		}
	}
	return append(events, ev)
}
func (b builder) functionCandidates(sig *types.Signature) []string {
	known := map[string]bool{}
	for _, d := range b.a.declarations {
		var candidate *types.Signature
		if d.object != nil {
			candidate, _ = d.object.Type().(*types.Signature)
		} else if lit, ok := d.node.(*ast.FuncLit); ok {
			candidate, _ = d.pkg.info.TypeOf(lit).(*types.Signature)
		}
		if candidate != nil && types.Identical(sig, candidate) {
			known[d.callable.ID] = true
		}
	}
	// Instantiated signatures are compiler-provided; never infer type arguments
	// from spelling or guess a generic function's specialization.
	for _, sf := range b.a.files {
		for ident, instance := range sf.pkg.info.Instances {
			if !types.Identical(sig, instance.Type) {
				continue
			}
			if fn, ok := sf.pkg.info.Uses[ident].(*types.Func); ok {
				if d := b.a.functions[fn.Origin()]; d != nil {
					known[d.callable.ID] = true
				}
			}
		}
	}
	for _, sf := range b.a.files {
		for _, selection := range sf.pkg.info.Selections {
			if !types.Identical(sig, selection.Type()) {
				continue
			}
			if fn, ok := selection.Obj().(*types.Func); ok {
				if d := b.a.functions[fn.Origin()]; d != nil {
					known[d.callable.ID] = true
				}
			}
		}
	}
	ids := make([]string, 0, len(known))
	for id := range known {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
func (b builder) interfaceCandidates(selection *types.Selection, method *types.Func) []string {
	iface, ok := selection.Recv().Underlying().(*types.Interface)
	if !ok {
		return nil
	}
	known := map[string]bool{}
	for _, d := range b.a.declarations {
		if d.object == nil || d.object.Name() != method.Name() {
			continue
		}
		sig, ok := d.object.Type().(*types.Signature)
		if !ok || sig.Recv() == nil {
			continue
		}
		recv := sig.Recv().Type()
		if !types.Implements(recv, iface) {
			if _, pointer := recv.(*types.Pointer); pointer || !types.Implements(types.NewPointer(recv), iface) {
				continue
			}
		}
		if types.Identical(sig, method.Type()) {
			known[d.callable.ID] = true
		}
	}
	// Compiler-observed instantiated receivers can implement an interface even
	// when their generic declaration's unsubstituted signature does not.
	for _, sf := range b.a.files {
		for _, value := range sf.pkg.info.Types {
			recv := value.Type
			if recv == nil {
				continue
			}
			if !types.Implements(recv, iface) {
				if _, named := recv.(*types.Named); !named {
					continue
				}
				recv = types.NewPointer(recv)
				if !types.Implements(recv, iface) {
					continue
				}
			}
			selected := types.NewMethodSet(recv).Lookup(method.Pkg(), method.Name())
			if selected != nil {
				if fn, ok := selected.Obj().(*types.Func); ok {
					if d := b.a.functions[fn.Origin()]; d != nil {
						known[d.callable.ID] = true
					}
				}
			}
		}
	}
	ids := make([]string, 0, len(known))
	for id := range known {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
func (b builder) block(list []ast.Stmt) ([]sequence.Event, bool) {
	var events []sequence.Event
	for _, stmt := range list {
		if b.a.ctx.Err() != nil {
			return events, true
		}
		part, terminal := b.stmt(stmt)
		events = append(events, part...)
		if terminal {
			return events, true
		}
	}
	return events, false
}
func (b builder) stmt(stmt ast.Stmt) ([]sequence.Event, bool) {
	if stmt == nil {
		return nil, false
	}
	switch s := stmt.(type) {
	case *ast.EmptyStmt:
		return nil, false
	case *ast.BlockStmt:
		return b.block(s.List)
	case *ast.ExprStmt:
		events := b.expr(s.X)
		if call, ok := unparen(s.X).(*ast.CallExpr); ok {
			if ident, ok := unparen(call.Fun).(*ast.Ident); ok {
				if builtin, ok := b.pkg.info.Uses[ident].(*types.Builtin); ok && builtin.Name() == "panic" {
					events = append(events, b.event(sequence.Return, s, "panic ends this path (deferred execution not simulated)"))
					return events, true
				}
			}
		}
		return events, false
	case *ast.AssignStmt:
		var events []sequence.Event
		for _, left := range s.Lhs {
			events = append(events, b.expr(left)...)
		}
		for _, right := range s.Rhs {
			events = append(events, b.expr(right)...)
		}
		return events, false
	case *ast.IncDecStmt:
		return b.expr(s.X), false
	case *ast.DeclStmt:
		var events []sequence.Event
		if decl, ok := s.Decl.(*ast.GenDecl); ok {
			for _, spec := range decl.Specs {
				if v, ok := spec.(*ast.ValueSpec); ok {
					for _, value := range v.Values {
						events = append(events, b.expr(value)...)
					}
				}
			}
		} else {
			return []sequence.Event{b.unsupported(s, "unsupported local declaration")}, true
		}
		return events, false
	case *ast.ReturnStmt:
		var events []sequence.Event
		for _, result := range s.Results {
			events = append(events, b.expr(result)...)
		}
		return append(events, b.event(sequence.Return, s, b.text(s))), true
	case *ast.GoStmt:
		return b.call(s.Call, sequence.Async), false
	case *ast.DeferStmt:
		return b.call(s.Call, sequence.Defer), false
	case *ast.SendStmt:
		events := b.expr(s.Chan)
		events = append(events, b.expr(s.Value)...)
		return append(events, b.event(sequence.Send, s, "send "+b.text(s.Chan)+" (communication peer unknown)")), false
	case *ast.IfStmt:
		events, _ := b.stmt(s.Init)
		events = append(events, b.expr(s.Cond)...)
		ev := b.event(sequence.Alt, s, b.text(s.Cond))
		var thenEnd, elseEnd bool
		ev.Children, thenEnd = b.block(s.Body.List)
		if s.Else != nil {
			ev.Exhaustive = true
			ev.Else, elseEnd = b.stmt(s.Else)
		}
		return append(events, ev), thenEnd && elseEnd
	case *ast.ForStmt:
		events, _ := b.stmt(s.Init)
		label := "for"
		if s.Cond != nil {
			label = "for " + b.text(s.Cond)
		}
		ev := b.event(sequence.Loop, s, label)
		ev.Children = b.expr(s.Cond)
		body, terminal := b.block(s.Body.List)
		ev.Children = append(ev.Children, body...)
		if !terminal {
			post, _ := b.stmt(s.Post)
			ev.Children = append(ev.Children, post...)
		}
		return append(events, ev), s.Cond == nil
	case *ast.RangeStmt:
		events := b.expr(s.X)
		ev := b.event(sequence.Loop, s, "range "+b.text(s.X))
		if typ := b.pkg.info.TypeOf(s.X); typ != nil {
			if _, ok := typ.Underlying().(*types.Signature); ok {
				return append(events, b.unsupported(s, "range-over-function iterator protocol is not projected")), true
			}
			if _, ok := typ.Underlying().(*types.Chan); ok {
				ev.Children = append(ev.Children, b.event(sequence.Receive, s.X, "range receive (communication peer unknown)"))
			}
		}
		ev.Children = append(ev.Children, b.expr(s.Key)...)
		ev.Children = append(ev.Children, b.expr(s.Value)...)
		body, _ := b.block(s.Body.List)
		ev.Children = append(ev.Children, body...)
		return append(events, ev), false
	case *ast.SwitchStmt:
		events, _ := b.stmt(s.Init)
		events = append(events, b.expr(s.Tag)...)
		label := "switch"
		if s.Tag != nil {
			label += " " + b.text(s.Tag)
		}
		ev := b.event(sequence.Alt, s, label)
		terminal, hasDefault := true, false
		for _, clause := range s.Body.List {
			c, ok := clause.(*ast.CaseClause)
			if !ok {
				ev.Branches = append(ev.Branches, sequence.Branch{Label: "invalid case", Events: []sequence.Event{b.unsupported(clause, "invalid switch case")}})
				continue
			}
			branch := sequence.Branch{Label: "default"}
			if len(c.List) == 0 {
				hasDefault = true
			} else {
				labels := []string{}
				for _, cond := range c.List {
					labels = append(labels, b.text(cond))
					part := b.event(sequence.Opt, cond, "evaluate case "+b.text(cond)+" if no preceding case expression matched")
					part.Children = b.expr(cond)
					if len(part.Children) > 0 {
						events = append(events, part)
					}
				}
				branch.Label = "case " + strings.Join(labels, ", ")
			}
			body, end := b.block(c.Body)
			branch.Events = body
			terminal = terminal && end
			ev.Branches = append(ev.Branches, branch)
		}
		ev.Exhaustive = hasDefault
		return append(events, ev), terminal && hasDefault
	case *ast.TypeSwitchStmt:
		events, _ := b.stmt(s.Init)
		assign, _ := b.stmt(s.Assign)
		events = append(events, assign...)
		ev := b.event(sequence.Alt, s, "type switch "+b.text(s.Assign))
		terminal, hasDefault := true, false
		for _, clause := range s.Body.List {
			c, ok := clause.(*ast.CaseClause)
			if !ok {
				ev.Branches = append(ev.Branches, sequence.Branch{Label: "invalid case", Events: []sequence.Event{b.unsupported(clause, "invalid type switch case")}})
				continue
			}
			label := "default"
			if len(c.List) == 0 {
				hasDefault = true
			} else {
				labels := []string{}
				for _, typ := range c.List {
					labels = append(labels, b.text(typ))
				}
				label = "case " + strings.Join(labels, ", ")
			}
			body, end := b.block(c.Body)
			terminal = terminal && end
			ev.Branches = append(ev.Branches, sequence.Branch{Label: label, Events: body})
		}
		ev.Exhaustive = hasDefault
		return append(events, ev), terminal && hasDefault
	case *ast.SelectStmt:
		return b.selectStmt(s)
	case *ast.BranchStmt:
		return []sequence.Event{b.unsupported(s, s.Tok.String()+" transfers control; subsequent execution is not inferred")}, true
	case *ast.LabeledStmt:
		return []sequence.Event{b.unsupported(s, "labeled control flow is not projected")}, true
	case *ast.BadStmt:
		return []sequence.Event{b.unsupported(s, "parser could not recover statement")}, true
	default:
		return []sequence.Event{b.unsupported(s, "unsupported Go statement")}, true
	}
}
func (b builder) selectStmt(s *ast.SelectStmt) ([]sequence.Event, bool) {
	if len(s.Body.List) == 0 {
		return []sequence.Event{b.unsupported(s, "select without cases blocks forever; no continuation is projected")}, true
	}
	var events []sequence.Event
	terminal := true
	ev := b.event(sequence.Alt, s, "select (one ready alternative; communication peer unknown)")
	ev.Exhaustive = true
	for _, node := range s.Body.List {
		c, ok := node.(*ast.CommClause)
		if !ok {
			events = append(events, b.unsupported(node, "invalid select clause"))
			continue
		}
		branch := sequence.Branch{Label: "default"}
		if c.Comm != nil {
			branch.Label = "case " + b.text(c.Comm)
			switch comm := c.Comm.(type) {
			case *ast.SendStmt:
				events = append(events, b.expr(comm.Chan)...)
				events = append(events, b.expr(comm.Value)...)
				branch.Events = append(branch.Events, b.event(sequence.Send, comm, "selected send "+b.text(comm.Chan)+" (communication peer unknown)"))
			case *ast.ExprStmt:
				if recv, ok := unparen(comm.X).(*ast.UnaryExpr); ok && recv.Op == token.ARROW {
					events = append(events, b.expr(recv.X)...)
					branch.Events = append(branch.Events, b.event(sequence.Receive, recv, "selected receive "+b.text(recv.X)+" (communication peer unknown)"))
				} else {
					branch.Events = append(branch.Events, b.unsupported(comm, "invalid select receive"))
				}
			case *ast.AssignStmt:
				if len(comm.Rhs) == 1 {
					if recv, ok := unparen(comm.Rhs[0]).(*ast.UnaryExpr); ok && recv.Op == token.ARROW {
						events = append(events, b.expr(recv.X)...)
						branch.Events = append(branch.Events, b.event(sequence.Receive, recv, "selected receive "+b.text(recv.X)+" (communication peer unknown)"))
						for _, left := range comm.Lhs {
							branch.Events = append(branch.Events, b.expr(left)...)
						}
					} else {
						branch.Events = append(branch.Events, b.unsupported(comm, "invalid select assignment"))
					}
				} else {
					branch.Events = append(branch.Events, b.unsupported(comm, "invalid select assignment"))
				}
			default:
				branch.Events = append(branch.Events, b.unsupported(comm, "unsupported select communication"))
			}
		}
		body, end := b.block(c.Body)
		terminal = terminal && end
		branch.Events = append(branch.Events, body...)
		ev.Branches = append(ev.Branches, branch)
	}
	return append(events, ev), terminal
}
