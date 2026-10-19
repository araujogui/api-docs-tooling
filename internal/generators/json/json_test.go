package json

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nodejs/doc-kit/internal/doc"
	gast "github.com/nodejs/doc-kit/internal/generators/ast"
	"github.com/nodejs/doc-kit/internal/markdown"
	"github.com/nodejs/doc-kit/internal/metadata"
	"github.com/nodejs/doc-kit/internal/omap"
	"github.com/nodejs/doc-kit/internal/queries"
)

var typeMap = map[string]string{
	"EventEmitter": "events.html#class-eventemitter",
	"Widget":       "widgets.html#class-widgetswidget",
}

var deps = func() dependencies {
	url := "https://github.com/example/widgets/blob/HEAD/"

	return dependencies{schemaURL: "https://example.com/api-doc.json", sourceURL: &url}
}()

// buildFixture runs a fixture through the metadata stage and builds its
// document.
func buildFixture(t *testing.T, name string) *omap.Map {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)

	// The AST stage links the stability prefix; mirror that
	if loc := queries.StabilityIndexPrefix.FindStringIndex(content); loc != nil {
		content = content[:loc[0]] + "[" + content[loc[0]:loc[1]] + "](" + gast.StabilityIndexURL + ")" + content[loc[1]:]
	}

	tree := markdown.Parse(content, markdown.Options{GFM: true, TypeAnnotations: true})
	entries, err := metadata.ParseAPIDoc(&doc.File{Path: "/" + strings.TrimSuffix(name, ".md"), Tree: tree}, typeMap)

	if err != nil {
		t.Fatal(err)
	}

	return BuildDocument(entries[0], entries, deps)
}

// snapshots reads the snapshots node:test recorded for the JavaScript
// version, by fixture name.
func snapshots(t *testing.T) map[string]string {
	t.Helper()

	data, err := os.ReadFile("testdata/generate.snapshot")
	if err != nil {
		t.Fatal(err)
	}

	out := map[string]string{}
	re := regexp.MustCompile("(?s)exports\\[`json > builds a valid document for (\\S+) 1`\\] = `\\n(.*?)\\n`;")

	for _, m := range re.FindAllStringSubmatch(string(data), -1) {
		// Template literal escapes
		out[m[1]] = strings.NewReplacer("\\`", "`", "\\${", "${", "\\\\", "\\").Replace(m[2])
	}

	return out
}

func TestBuildDocument(t *testing.T) {
	expected := snapshots(t)

	fixtures, _ := filepath.Glob("testdata/*.md")

	if len(fixtures) == 0 || len(expected) != len(fixtures) {
		t.Fatalf("%d fixtures, %d snapshots", len(fixtures), len(expected))
	}

	for _, path := range fixtures {
		name := filepath.Base(path)

		t.Run(name, func(t *testing.T) {
			got, err := omap.Stringify(buildFixture(t, name), 2)
			if err != nil {
				t.Fatal(err)
			}

			if string(got) != expected[name] {
				t.Errorf("document differs from the snapshot:\n%s", got)
			}
		})
	}
}

func TestResolveSchemaURL(t *testing.T) {
	if got := ResolveSchemaURL(map[string]any{"schemaURL": SchemaURL}); got != "https://doc-kit.nodejs.org/schemas/api-doc/"+SchemaVersion+".json" {
		t.Errorf("got %q", got)
	}
}
