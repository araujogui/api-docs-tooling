// Package addonverify is the generator extracting the C++ addon examples of
// `doc/api/addons.md` into buildable file trees, for Node.js to compile and
// run them.
package addonverify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/nodejs/doc-kit/internal/doc"
	"github.com/nodejs/doc-kit/internal/fsutil"
	"github.com/nodejs/doc-kit/internal/generators"
	_ "github.com/nodejs/doc-kit/internal/generators/metadata"
	"github.com/nodejs/doc-kit/internal/jsre"
	"github.com/nodejs/doc-kit/internal/unist"
)

func init() {
	generators.Register(&generators.Generator{
		Name:        "addon-verify",
		Description: "Generates a file list from code blocks extracted from `doc/api/addons.md` to facilitate C++ compilation and JavaScript runtime validations",
		DependsOn:   "metadata",
		Public:      true,
		Generate:    generate,
	})
}

// extractFilename matches a code block's leading `// file.cc` comment.
var extractFilename = jsre.MustCompile(`^//\s+(.*\.(?:cc|h|js))[\r\n]`)

// File is a generated file.
type File struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

var (
	escapedNewline = regexp.MustCompile("\\\\\n[ \t]*")
	indentedLine   = jsre.MustCompile(`^(\s+)\S+`)
	hexEscape      = regexp.MustCompile(`\\x([\da-fA-F]{2})`)
	codePointEsc   = regexp.MustCompile(`\\u\{([\da-fA-F]{1,6})\}`)
	unicodeEscape  = regexp.MustCompile(`\\u([\da-fA-F]{4})`)
)

// dedent interpolates a template like the `dedent` tag: the common
// indentation of its lines (values included) is stripped, it is trimmed, and
// escape sequences (in the values too) are decoded.
func dedent(raw []string, values ...string) string {
	var b strings.Builder

	for i, part := range raw {
		part = escapedNewline.ReplaceAllString(part, "")
		part = strings.NewReplacer("\\`", "`", `\$`, "$", `\{`, "{").Replace(part)
		b.WriteString(part)

		if i < len(values) {
			b.WriteString(values[i])
		}
	}

	result := b.String()
	lines := strings.Split(result, "\n")
	mindent := 0

	for _, l := range lines {
		if m := indentedLine.FindStringSubmatch(l); m != nil {
			indent := len(utf16.Encode([]rune(m[1])))

			if mindent == 0 || indent < mindent {
				mindent = indent
			}
		}
	}

	if mindent > 0 {
		for i, l := range lines {
			if strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t") {
				units := utf16.Encode([]rune(l))
				lines[i] = string(utf16.Decode(units[min(mindent, len(units)):]))
			}
		}

		result = strings.Join(lines, "\n")
	}

	result = jsre.Trim(result)
	result = strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\r`, "\r", `\v`, "\v", `\b`, "\b", `\f`, "\f", `\0`, "\x00").Replace(result)

	decode := func(re *regexp.Regexp, s string) string {
		return re.ReplaceAllStringFunc(s, func(m string) string {
			n, _ := strconv.ParseUint(re.FindStringSubmatch(m)[1], 16, 32)

			return string(rune(n))
		})
	}

	result = decode(hexEscape, result)
	result = decode(codePointEsc, result)

	return decode(unicodeEscape, result)
}

// updateJSRequirePaths points a test file at the build tree.
func updateJSRequirePaths(content string) string {
	return dedent([]string{"\n    'use strict';\n    const common = require('../../common');\n    ", ""},
		jsre.Replace(content, "'./build/Release/addon'", "`./build/${common.buildType}/addon`"))
}

// createBindingGyp creates the binding.gyp compiling the addon's sources.
func createBindingGyp(sources []string) string {
	config := map[string]any{
		"targets": []any{
			struct {
				TargetName string   `json:"target_name"`
				Sources    []string `json:"sources"`
				Includes   []string `json:"includes"`
			}{"addon", sources, []string{"../common.gypi"}},
		},
	}

	data, _ := json.Marshal(config)

	return string(data)
}

// GenerateFileList returns a section's files, with its binding.gyp.
func GenerateFileList(blocks []File) []File {
	files := make([]File, 0, len(blocks)+1)
	names := make([]string, 0, len(blocks))

	for _, b := range blocks {
		content := b.Content

		if b.Name == "test.js" {
			content = updateJSRequirePaths(content)
		}

		files = append(files, File{Name: b.Name, Content: content})
		names = append(names, b.Name)
	}

	return append(files, File{Name: "binding.gyp", Content: createBindingGyp(names)})
}

// IsBuildableSection reports whether a section has a C++ and a JavaScript
// file.
func IsBuildableSection(blocks []File) bool {
	cc, js := false, false

	for _, b := range blocks {
		cc = cc || strings.HasSuffix(b.Name, ".cc")
		js = js || strings.HasSuffix(b.Name, ".js")
	}

	return cc && js
}

var (
	whitespace = jsre.MustCompile(`\s`)
	nonWord    = regexp.MustCompile(`\W`)
)

// NormalizeSectionName turns a section's name into a folder name.
func NormalizeSectionName(name string) string {
	return nonWord.ReplaceAllString(whitespace.ReplaceAllString(strings.ToLower(name), "_"), "")
}

// SectionFolderName prefixes a section's folder with its 1-based index.
func SectionFolderName(name string, index int) string {
	return fmt.Sprintf("%02d_%s", index+1, name)
}

func generate(ctx *generators.Context, input any) (any, error) {
	var order []string
	sections := map[string][]File{}

	for _, e := range input.([]*doc.Entry) {
		name := e.HeadingData().Name

		unist.Visit(e.Content, unist.IsType("code"), func(n *unist.Node, _ int, _ *unist.Node) unist.Step {
			m := extractFilename.FindStringSubmatch(n.Value)

			if m == nil {
				return unist.Next
			}

			if _, ok := sections[name]; !ok {
				order = append(order, name)
			}

			sections[name] = append(sections[name], File{Name: m[1], Content: n.Value})

			return unist.Next
		})
	}

	var all [][]File
	index := 0

	for _, name := range order {
		blocks := sections[name]

		if !IsBuildableSection(blocks) {
			continue
		}

		files := GenerateFileList(blocks)
		all = append(all, files)

		if out := ctx.Options.String("output"); out != "" {
			folder := filepath.Join(out, SectionFolderName(NormalizeSectionName(name), index))

			if err := os.MkdirAll(folder, 0o755); err != nil {
				return nil, err
			}

			for _, f := range files {
				if err := fsutil.WriteFile(filepath.Join(folder, f.Name), []byte(f.Content)); err != nil {
					return nil, err
				}
			}
		}

		index++
	}

	return all, nil
}
