// Package apilinks is the generator mapping the publicly accessible
// functions of Node.js to their source locations (`apilinks.json`).
package apilinks

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/file"
	"github.com/nodejs/doc-kit/internal/config"
	"github.com/nodejs/doc-kit/internal/fsutil"
	"github.com/nodejs/doc-kit/internal/generators"
	"github.com/nodejs/doc-kit/internal/generators/astjs"
	"github.com/nodejs/doc-kit/internal/omap"
)

func init() {
	generators.Register(&generators.Generator{
		Name:        "api-links",
		Description: "Creates a mapping of publicly accessible functions to their source locations in the Node.js repository.",
		DependsOn:   "ast-js",
		Public:      true,
		Defaults: func(map[string]any) map[string]any {
			return map[string]any{"sourceURL": config.GitHubBlobURL + "lib/{fileName}"}
		},
		Generate: generate,
	})
}

// undef stands for JavaScript's `undefined`, where a name is missing (it
// prints as "undefined", as in a template string).
const undef = "\x00undefined"

func show(name string) string {
	if name == undef {
		return "undefined"
	}

	return name
}

// exports are what a program exports: constructors, other identifiers, and
// identifiers exported under their own name, by the key they are linked as.
type exports struct {
	ctors       []string
	identifiers []string
	indirects   *omap.Map
}

func newExports() *exports {
	return &exports{indirects: omap.New()}
}

// nameOf is an identifier's name.
func nameOf(e ast.Node) string {
	if id, ok := e.(*ast.Identifier); ok && id != nil {
		return id.Name.String()
	}

	return undef
}

// isMember tests for member expressions.
func isMember(e ast.Node) bool {
	switch e.(type) {
	case *ast.DotExpression, *ast.BracketExpression, *ast.PrivateDotExpression:
		return true
	}

	return false
}

// memberObject is a member expression's object.
func memberObject(e ast.Node) ast.Node {
	switch m := e.(type) {
	case *ast.DotExpression:
		return m.Left
	case *ast.BracketExpression:
		return m.Left
	case *ast.PrivateDotExpression:
		return m.Left
	}

	return nil
}

// propertyName is a member expression's property's name.
func propertyName(e ast.Node) string {
	switch m := e.(type) {
	case *ast.DotExpression:
		return m.Identifier.Name.String()
	case *ast.PrivateDotExpression:
		return m.Identifier.Name.String()
	case *ast.BracketExpression:
		return nameOf(m.Member)
	}

	return undef
}

func isConstructorName(name string) bool {
	return name != undef && name != "" && name[0] >= 'A' && name[0] <= 'Z'
}

type linker struct {
	program  *astjs.Program
	basename string
	lines    *omap.Map
}

func (l *linker) line(idx file.Idx) int {
	return l.program.Line(idx)
}

func (l *linker) set(key string, line int) {
	l.lines.Set(show(key), line)
}

func (l *linker) handleExpression(node *ast.ExpressionStatement) *exports {
	expr, ok := node.Expression.(*ast.AssignExpression)

	if !ok || !isMember(expr.Left) {
		return nil
	}

	var lhs, rhs ast.Node = expr.Left, expr.Right
	line := l.line(expr.Idx0())

	if isMember(memberObject(lhs)) {
		lhs = memberObject(lhs)
	}

	out := newExports()
	object := nameOf(memberObject(lhs))
	property := propertyName(lhs)

	switch {
	case object == "exports":
		// An assignment to a property of `exports`
		switch r := rhs.(type) {
		case *ast.FunctionLiteral:
			l.set(l.basename+"."+show(property), line)
		case *ast.Identifier:
			if r.Name.String() == property {
				out.indirects.Set(show(property), l.basename+"."+show(property))
			}
		default:
			if property != undef {
				out.identifiers = append(out.identifiers, property)
			}
		}
	case object == "module" && property == "exports":
		// An assignment to `module.exports` as a whole
		for {
			a, ok := rhs.(*ast.AssignExpression)

			if !ok {
				break
			}

			rhs = a.Right
		}

		switch r := rhs.(type) {
		case *ast.NewExpression:
			out.ctors = append(out.ctors, nameOf(r.Callee))
		case *ast.ObjectLiteral:
			for _, p := range r.Value {
				var value ast.Node

				switch prop := p.(type) {
				case *ast.PropertyShort:
					value = &prop.Name
				case *ast.PropertyKeyed:
					value = prop.Value
				default:
					continue
				}

				switch v := value.(type) {
				case *ast.Identifier:
					name := v.Name.String()
					out.identifiers = append(out.identifiers, name)

					if isConstructorName(name) {
						out.ctors = append(out.ctors, name)
					}
				case *ast.CallExpression:
					// Exports wrapped in `deprecate()`
					if nameOf(v.Callee) != "deprecate" {
						continue
					}

					if len(v.ArgumentList) > 0 {
						out.identifiers = append(out.identifiers, nameOf(v.ArgumentList[0]))
					} else {
						out.identifiers = append(out.identifiers, undef)
					}
				}
			}
		case *ast.Identifier:
			out.identifiers = append(out.identifiers, r.Name.String())
		}
	}

	return out
}

func (l *linker) handleVariableDeclaration(line int, bindings []*ast.Binding) *exports {
	out := newExports()

	for _, b := range bindings {
		var lhs ast.Node = b.Initializer

		for {
			a, ok := lhs.(*ast.AssignExpression)

			if !ok {
				break
			}

			lhs = a.Left
		}

		if lhs == nil || !isMember(lhs) {
			continue
		}

		switch nameOf(memberObject(lhs)) {
		case "exports":
			l.set(l.basename+"."+show(propertyName(lhs)), line)
		case "module":
			if propertyName(lhs) != "exports" {
				continue
			}

			name := nameOf(b.Target)
			out.ctors = append(out.ctors, name)
			l.set(name, line)
		}
	}

	return out
}

// declarations returns a variable declaration's line and bindings.
func declarations(n ast.Node, program *astjs.Program) (int, []*ast.Binding, bool) {
	switch d := n.(type) {
	case *ast.VariableStatement:
		return program.Line(d.Var), d.List, true
	case *ast.LexicalDeclaration:
		return program.Line(d.Idx), d.List, true
	case *ast.ForLoopInitializerVarDeclList:
		return program.Line(d.Var), d.List, true
	case *ast.ForLoopInitializerLexicalDecl:
		return program.Line(d.LexicalDeclaration.Idx), d.LexicalDeclaration.List, true
	}

	return 0, nil, false
}

// extractExports finds what a program exports, from its assignments to
// `module.exports` and `exports` throughout.
func (l *linker) extractExports() *exports {
	all := newExports()

	walk(l.program.Program, func(n ast.Node) {
		var out *exports

		if s, ok := n.(*ast.ExpressionStatement); ok {
			out = l.handleExpression(s)
		} else if line, bindings, ok := declarations(n, l.program); ok {
			out = l.handleVariableDeclaration(line, bindings)
		}

		if out != nil {
			all.ctors = append(all.ctors, out.ctors...)
			all.identifiers = append(all.identifiers, out.identifiers...)

			for _, k := range out.indirects.Keys() {
				v, _ := out.indirects.Get(k)
				all.indirects.Set(k, v)
			}
		}
	})

	return all
}

func (l *linker) handleAssignment(node *ast.ExpressionStatement, ex *exports) {
	expr, ok := node.Expression.(*ast.AssignExpression)

	if !ok || !isMember(expr.Left) {
		return
	}

	lhs, rhs := expr.Left, expr.Right

	var object ast.Node
	var objectName string

	switch obj := memberObject(lhs); {
	case isMember(obj):
		if propertyName(obj) != "prototype" {
			return
		}

		// `ClassName.prototype.method = ...`
		object = memberObject(obj)

		if name := nameOf(object); name != undef && name != "" {
			objectName = name
		} else if isMember(object) {
			objectName = nameOf(memberObject(object))
		} else {
			return
		}

		if objectName == undef {
			return
		}

		objectName = strings.ToLower(objectName)

		// Some of the docs refer to buffers as `buf`
		if objectName == "buffer" {
			objectName = "buf"
		}
	default:
		id, ok := obj.(*ast.Identifier)

		if !ok {
			return
		}

		object = id
		objectName = id.Name.String()
	}

	if !slices.Contains(ex.ctors, nameOf(object)) {
		return
	}

	property := propertyName(lhs)
	name := objectName + "." + show(property)

	if _, computed := lhs.(*ast.BracketExpression); computed {
		name = objectName + "[" + show(property) + "]"
	}

	l.set(name, l.line(expr.Idx0()))

	if property == nameOf(rhs) {
		ex.indirects.Set(show(nameOf(rhs)), name)
	}
}

func (l *linker) handleFunction(fn *ast.FunctionLiteral, ex *exports) {
	if fn.Name == nil || !slices.Contains(ex.identifiers, fn.Name.Name.String()) {
		return
	}

	// Internal functions aren't documented
	if strings.HasPrefix(l.basename, "_") {
		return
	}

	l.set(l.basename+"."+fn.Name.Name.String(), l.line(fn.Idx0()))
}

// keyName is a class member's key's name, as ESTree has it.
func keyName(key ast.Expression, computed bool) string {
	switch k := key.(type) {
	case *ast.Identifier:
		return k.Name.String()
	case *ast.PrivateIdentifier:
		return k.Name.String()
	case *ast.StringLiteral:
		// Identifier keys are string literals here, without quotes
		if !computed && !strings.HasPrefix(k.Literal, "'") && !strings.HasPrefix(k.Literal, "\"") {
			return k.Value.String()
		}
	}

	return undef
}

func (l *linker) handleClass(class *ast.ClassLiteral, ex *exports) {
	if class.Name == nil {
		return
	}

	className := class.Name.Name.String()

	if !slices.Contains(ex.ctors, className) {
		return
	}

	// WASI -> wASI, Agent -> agent
	name := strings.ToLower(className[:1]) + className[1:]

	l.set(className, l.line(class.Idx0()))

	for _, el := range class.Body {
		m, ok := el.(*ast.MethodDefinition)

		if !ok {
			continue
		}

		var key string

		if s, isString := m.Key.(*ast.StringLiteral); isString && !m.Computed && !m.Static && s.Value.String() == "constructor" {
			key = "new " + className
		} else {
			key = name + "." + show(keyName(m.Key, m.Computed))
		}

		l.set(key, l.line(m.Idx))
	}
}

func (l *linker) findDefinitions(ex *exports) {
	walk(l.program.Program, func(n ast.Node) {
		switch d := n.(type) {
		case *ast.ExpressionStatement:
			l.handleAssignment(d, ex)
		case *ast.FunctionDeclaration:
			l.handleFunction(d.Function, ex)
		case *ast.ClassDeclaration:
			l.handleClass(d.Class, ex)
		}
	})
}

func (l *linker) checkIndirectReferences(ex *exports) {
	if ex.indirects.Len() == 0 {
		return
	}

	walk(l.program.Program, func(n ast.Node) {
		d, ok := n.(*ast.FunctionDeclaration)

		if !ok || d.Function.Name == nil {
			return
		}

		if key, ok := ex.indirects.Value(d.Function.Name.Name.String()).(string); ok {
			l.lines.Set(key, l.line(d.Function.Idx0()))
		}
	})
}

// Links maps a program's public definitions to their line numbers.
func Links(program *astjs.Program) *omap.Map {
	fileName := filepath.Base(program.Path)
	l := &linker{program: program, basename: fsutil.WithExt(fileName, ""), lines: omap.New()}

	ex := l.extractExports()
	l.findDefinitions(ex)
	l.checkIndirectReferences(ex)

	return l.lines
}

func generate(ctx *generators.Context, input any) (any, error) {
	definitions := omap.New()

	for _, program := range input.([]*astjs.Program) {
		lines := Links(program)
		url := config.Populate(ctx.Options.String("sourceURL"), ctx.Options.With("fileName", filepath.Base(program.Path)))

		for _, key := range lines.Keys() {
			definitions.Set(key, fmt.Sprintf("%s#L%v", url, lines.Value(key)))
		}
	}

	if out := ctx.Options.String("output"); out != "" {
		indent := 2

		if ctx.Options.Bool("minify") {
			indent = 0
		}

		data, err := omap.Stringify(definitions, indent)
		if err != nil {
			return nil, err
		}

		if err := fsutil.WriteFile(filepath.Join(out, "apilinks.json"), data); err != nil {
			return nil, err
		}
	}

	return definitions, nil
}
