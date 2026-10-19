// Package manpage is the generator of the Node.js man page (see
// https://man.openbsd.org/mdoc.7 for the format).
package manpage

import (
	_ "embed"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nodejs/doc-kit/internal/doc"
	"github.com/nodejs/doc-kit/internal/fsutil"
	"github.com/nodejs/doc-kit/internal/generators"
	_ "github.com/nodejs/doc-kit/internal/generators/metadata"
	"github.com/nodejs/doc-kit/internal/jsre"
	"github.com/nodejs/doc-kit/internal/unist"
)

//go:embed template.1
var defaultTemplate string

func init() {
	generators.Register(&generators.Generator{
		Name:        "man-page",
		Description: "Generates the Node.js man-page.",
		DependsOn:   "metadata",
		Public:      true,
		Defaults: func(map[string]any) map[string]any {
			return map[string]any{
				"fileName":             "node.1",
				"cliOptionsHeaderSlug": "options",
				"envVarsHeaderSlug":    "environment-variables-1",
				"templatePath":         "",
			}
		},
		Generate: generate,
	})
}

var backslash = strings.NewReplacer(`\`, `\\`)

// ConvertNodeToMandoc converts an mdast node into mandoc markup; unsupported
// nodes (HTML, blockquotes, ...) are dropped.
func ConvertNodeToMandoc(n *unist.Node, isListItem bool) string {
	children := func(sep string, listItem bool) string {
		parts := make([]string, len(n.Children))

		for i, c := range n.Children {
			parts[i] = ConvertNodeToMandoc(c, listItem)
		}

		return strings.Join(parts, sep)
	}

	escaped := func() string { return backslash.Replace(n.Value) }

	switch n.Type {
	case "root":
		return children("\n", false)
	case "heading":
		return ".Sh " + children("", false)
	case "link", "paragraph", "listItem":
		prefix := ""

		if isListItem && n.Type == "listItem" {
			prefix = ".It\n"
		}

		return prefix + children("", false)
	case "text":
		return escaped()
	case "inlineCode":
		return `\fB` + escaped() + `\fR`
	case "typeAnnotation":
		return `\fB{` + escaped() + `}\fR`
	case "strong":
		return `\fB` + children("", false) + `\fR`
	case "code":
		return ".Bd -literal\n" + escaped() + "\n.Ed"
	case "list":
		return ".Bl -bullet\n" + children("\n", true) + "\n.El"
	case "emphasis":
		return `\fI` + children("", false) + `\fR`
	}

	return ""
}

var separator = regexp.MustCompile(`[= ]`)

// FlagValueToMandoc formats a flag's value (after `=` or a space).
func FlagValueToMandoc(flag string) string {
	sep := separator.FindString(flag)

	if sep == "" {
		return ""
	}

	parts := strings.Split(flag, sep)
	value := parts[1]

	// A value ending in `]` is optional
	if strings.HasSuffix(value, "]") {
		value = "[" + value
	}

	if value == "" {
		return ""
	}

	prefix := " Ns = Ns"

	if sep == " " {
		prefix = ""
	}

	return prefix + " Ar " + value
}

var flagName = regexp.MustCompile(`\[?[= ]`)

func formatFlag(flag string) string {
	name := flagName.Split(flag, -1)[0]

	if name != "" {
		name = name[1:]
	}

	return "Fl " + name + FlagValueToMandoc(flag)
}

// ConvertOptionToMandoc converts a CLI option entry.
func ConvertOptionToMandoc(e *doc.Entry) string {
	flags := strings.Split(strings.ReplaceAll(e.HeadingData().Text, "`", ""), ", ")

	for i, f := range flags {
		flags[i] = formatFlag(f)
	}

	formatted := strings.TrimSpace(strings.Join(flags, " , "))

	// Without the heading itself
	if len(e.Content.Children) > 0 {
		e.Content.Children = e.Content.Children[1:]
	}

	return ".It " + formatted + "\n" + strings.TrimSpace(ConvertNodeToMandoc(e.Content, false)) + "\n.\n"
}

// ConvertEnvVarToMandoc converts an environment variable entry.
func ConvertEnvVarToMandoc(e *doc.Entry) string {
	parts := strings.Split(strings.ReplaceAll(e.HeadingData().Text, "`", ""), "=")
	value := ""

	if len(parts) > 1 && parts[1] != "" {
		value = " Ar " + parts[1]
	}

	if len(e.Content.Children) > 0 {
		e.Content.Children = e.Content.Children[1:]
	}

	return ".It Ev " + parts[0] + value + "\n" + ConvertNodeToMandoc(e.Content, false) + "\n.\n"
}

// extract converts the depth-3 entries in [start, end).
func extract(entries []*doc.Entry, start, end int, convert func(*doc.Entry) string) string {
	var b strings.Builder

	if end < 0 {
		end = max(len(entries)+end, 0)
	}

	for i := max(start, 0); i < end && i < len(entries); i++ {
		if entries[i].Heading.Depth == 3 {
			b.WriteString(convert(entries[i]))
		}
	}

	return b.String()
}

func findIndex(entries []*doc.Entry, test func(int, *doc.Entry) bool) int {
	for i, e := range entries {
		if test(i, e) {
			return i
		}
	}

	return -1
}

func generate(ctx *generators.Context, input any) (any, error) {
	entries := doc.CloneEntries(input.([]*doc.Entry))
	o := ctx.Options

	optionsStart := findIndex(entries, func(_ int, e *doc.Entry) bool { return e.HeadingData().Slug == o.String("cliOptionsHeaderSlug") })
	envStart := findIndex(entries, func(_ int, e *doc.Entry) bool { return e.HeadingData().Slug == o.String("envVarsHeaderSlug") })

	// The first heading shallower than 3 after the environment variables
	envEnd := findIndex(entries, func(i int, e *doc.Entry) bool { return e.Heading.Depth < 3 && i > envStart })

	options := extract(entries, optionsStart+1, envStart, ConvertOptionToMandoc)
	env := extract(entries, envStart+1, envEnd, ConvertEnvVarToMandoc)

	template := defaultTemplate

	if path := o.String("templatePath"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		template = string(data)
	}

	filled := jsre.Replace(jsre.Replace(template, "__OPTIONS__", options), "__ENVIRONMENT__", env)

	if out := o.String("output"); out != "" {
		if err := fsutil.WriteFile(filepath.Join(out, o.String("fileName")), []byte(filled)); err != nil {
			return nil, err
		}
	}

	return filled, nil
}
