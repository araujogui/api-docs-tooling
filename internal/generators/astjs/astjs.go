// Package astjs is the generator parsing the JavaScript sources given in the
// input. It is separate from the Markdown parsing, which most runs need, so
// the sources are only parsed when a generator asks for them.
package astjs

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/file"
	"github.com/dop251/goja/parser"
	"github.com/nodejs/doc-kit/internal/generators"
	gast "github.com/nodejs/doc-kit/internal/generators/ast"
)

// Program is a parsed JavaScript file.
type Program struct {
	*ast.Program
	// Path is the file's path.
	Path string
	// Files resolves the program's positions.
	Files *file.FileSet
}

// Line returns the line of a position.
func (p *Program) Line(idx file.Idx) int {
	return p.Files.Position(idx).Line
}

func init() {
	generators.Register(&generators.Generator{
		Name:        "ast-js",
		Description: "Parses Javascript source files passed into the input.",
		Generate:    generate,
	})
}

// ParseFile parses a JavaScript file.
func ParseFile(path string) (*Program, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	files := &file.FileSet{}

	program, err := parser.ParseFile(files, path, string(src), 0)

	// Scripts may return at their top level (CommonJS modules run in a
	// function): parse those inside one, keeping their lines where they are
	if err != nil && strings.Contains(err.Error(), "Illegal return statement") {
		files = &file.FileSet{}
		program, err = parser.ParseFile(files, path, wrapInFunction(string(src)), 0)
	}

	if err != nil {
		return nil, err
	}

	return &Program{Program: program, Path: path, Files: files}, nil
}

// wrapInFunction wraps a script in a function, on its first and last lines.
func wrapInFunction(src string) string {
	// A hashbang must stay first
	if strings.HasPrefix(src, "#!") {
		end := strings.IndexByte(src, '\n')

		if end < 0 {
			return src
		}

		return src[:end+1] + "(function () {" + src[end+1:] + "\n})();"
	}

	return "(function () {" + src + "\n})();"
}

func generate(ctx *generators.Context, _ any) (any, error) {
	matches, err := gast.Glob(ctx.Options.Strings("input"), ctx.Options.Strings("ignore"))
	if err != nil {
		return nil, err
	}

	var paths []string

	for _, m := range matches {
		if filepath.Ext(m[0]) == ".js" {
			paths = append(paths, m[0])
		}
	}

	programs := make([]*Program, len(paths))

	err = ctx.Run.Parallel(len(paths), func(i int) error {
		p, err := ParseFile(paths[i])
		programs[i] = p

		return err
	})

	return programs, err
}
