package apilinks

import (
	"github.com/dop251/goja/ast"
)

// walk visits a node and its descendants in preorder, in source order, as
// `estree-util-visit` does over an ESTree.
func walk(node ast.Node, visit func(ast.Node)) {
	if isNil(node) {
		return
	}

	visit(node)

	w := func(children ...ast.Node) {
		for _, c := range children {
			walk(c, visit)
		}
	}

	switch n := node.(type) {
	case *ast.Program:
		for _, s := range n.Body {
			w(s)
		}
	case *ast.BlockStatement:
		for _, s := range n.List {
			w(s)
		}
	case *ast.ExpressionStatement:
		w(n.Expression)
	case *ast.VariableStatement:
		walkBindings(n.List, visit)
	case *ast.LexicalDeclaration:
		walkBindings(n.List, visit)
	case *ast.FunctionDeclaration:
		w(n.Function)
	case *ast.ClassDeclaration:
		w(n.Class)
	case *ast.IfStatement:
		w(n.Test, n.Consequent, n.Alternate)
	case *ast.ForStatement:
		w(n.Initializer, n.Test, n.Update, n.Body)
	case *ast.ForLoopInitializerExpression:
		w(n.Expression)
	case *ast.ForLoopInitializerVarDeclList:
		walkBindings(n.List, visit)
	case *ast.ForLoopInitializerLexicalDecl:
		walkBindings(n.LexicalDeclaration.List, visit)
	case *ast.ForInStatement:
		w(n.Into, n.Source, n.Body)
	case *ast.ForOfStatement:
		w(n.Into, n.Source, n.Body)
	case *ast.ForIntoVar:
		walkBindings([]*ast.Binding{n.Binding}, visit)
	case *ast.ForDeclaration:
		w(n.Target)
	case *ast.ForIntoExpression:
		w(n.Expression)
	case *ast.WhileStatement:
		w(n.Test, n.Body)
	case *ast.DoWhileStatement:
		w(n.Body, n.Test)
	case *ast.ReturnStatement:
		w(n.Argument)
	case *ast.ThrowStatement:
		w(n.Argument)
	case *ast.TryStatement:
		w(n.Body, n.Catch, n.Finally)
	case *ast.CatchStatement:
		w(n.Parameter, n.Body)
	case *ast.SwitchStatement:
		w(n.Discriminant)

		for _, c := range n.Body {
			w(c)
		}
	case *ast.CaseStatement:
		w(n.Test)

		for _, s := range n.Consequent {
			w(s)
		}
	case *ast.LabelledStatement:
		w(n.Statement)
	case *ast.WithStatement:
		w(n.Object, n.Body)
	case *ast.AssignExpression:
		w(n.Left, n.Right)
	case *ast.BinaryExpression:
		w(n.Left, n.Right)
	case *ast.ConditionalExpression:
		w(n.Test, n.Consequent, n.Alternate)
	case *ast.CallExpression:
		w(n.Callee)

		for _, a := range n.ArgumentList {
			w(a)
		}
	case *ast.NewExpression:
		w(n.Callee)

		for _, a := range n.ArgumentList {
			w(a)
		}
	case *ast.DotExpression:
		w(n.Left)
	case *ast.PrivateDotExpression:
		w(n.Left)
	case *ast.BracketExpression:
		w(n.Left, n.Member)
	case *ast.UnaryExpression:
		w(n.Operand)
	case *ast.SequenceExpression:
		for _, e := range n.Sequence {
			w(e)
		}
	case *ast.ArrayLiteral:
		for _, e := range n.Value {
			w(e)
		}
	case *ast.ArrayPattern:
		for _, e := range n.Elements {
			w(e)
		}

		w(n.Rest)
	case *ast.ObjectLiteral:
		for _, p := range n.Value {
			w(p)
		}
	case *ast.ObjectPattern:
		for _, p := range n.Properties {
			w(p)
		}

		w(n.Rest)
	case *ast.PropertyKeyed:
		w(n.Key, n.Value)
	case *ast.PropertyShort:
		w(&n.Name, n.Initializer)
	case *ast.SpreadElement:
		w(n.Expression)
	case *ast.TemplateLiteral:
		w(n.Tag)

		for _, e := range n.Expressions {
			w(e)
		}
	case *ast.FunctionLiteral:
		if n.ParameterList != nil {
			walkBindings(n.ParameterList.List, visit)
			w(n.ParameterList.Rest)
		}

		w(n.Body)
	case *ast.ArrowFunctionLiteral:
		if n.ParameterList != nil {
			walkBindings(n.ParameterList.List, visit)
			w(n.ParameterList.Rest)
		}

		w(n.Body)
	case *ast.ExpressionBody:
		w(n.Expression)
	case *ast.ClassLiteral:
		w(n.SuperClass)

		for _, e := range n.Body {
			w(e)
		}
	case *ast.FieldDefinition:
		w(n.Key, n.Initializer)
	case *ast.MethodDefinition:
		w(n.Key, n.Body)
	case *ast.ClassStaticBlock:
		w(n.Block)
	case *ast.YieldExpression:
		w(n.Argument)
	case *ast.AwaitExpression:
		w(n.Argument)
	case *ast.OptionalChain:
		w(n.Expression)
	case *ast.Optional:
		w(n.Expression)
	}
}

func walkBindings(bindings []*ast.Binding, visit func(ast.Node)) {
	for _, b := range bindings {
		if b == nil {
			continue
		}

		walk(b.Target, visit)
		walk(b.Initializer, visit)
	}
}

// isNil reports whether an interface holds nothing (or a nil pointer).
func isNil(n ast.Node) bool {
	if n == nil {
		return true
	}

	switch v := n.(type) {
	case *ast.BlockStatement:
		return v == nil
	case *ast.CatchStatement:
		return v == nil
	case *ast.FunctionLiteral:
		return v == nil
	case *ast.ClassLiteral:
		return v == nil
	case *ast.Identifier:
		return v == nil
	case *ast.ExpressionBody:
		return v == nil
	}

	return false
}
