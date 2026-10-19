package apilinks

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/nodejs/doc-kit/internal/generators/astjs"
	"github.com/nodejs/doc-kit/internal/omap"
)

// snapshots reads the outputs node:test recorded for the JavaScript
// version, by fixture name.
func snapshots(t *testing.T) map[string]string {
	t.Helper()

	data, err := os.ReadFile("testdata/fixtures.snapshot")
	if err != nil {
		t.Fatal(err)
	}

	out := map[string]string{}
	re := regexp.MustCompile("(?s)exports\\[`api links > should work correctly for all fixtures > (\\S+) 1`\\] = `\\n(.*?)\\n`;")

	for _, m := range re.FindAllStringSubmatch(string(data), -1) {
		out[m[1]] = m[2]
	}

	return out
}

func TestLinks(t *testing.T) {
	expected := snapshots(t)
	fixtures, _ := filepath.Glob("testdata/*.js")

	if len(fixtures) == 0 || len(fixtures) != len(expected) {
		t.Fatalf("%d fixtures, %d snapshots", len(fixtures), len(expected))
	}

	for _, path := range fixtures {
		name := filepath.Base(path)

		t.Run(name, func(t *testing.T) {
			program, err := astjs.ParseFile(path)
			if err != nil {
				t.Fatal(err)
			}

			lines := Links(program)
			got := omap.New()

			for _, k := range lines.Keys() {
				got.Set(k, fmt.Sprintf("lib/%s#L%v", name, lines.Value(k)))
			}

			data, _ := omap.Stringify(got, 2)

			if string(data) != expected[name] {
				t.Errorf("got\n%s\nwant\n%s", data, expected[name])
			}
		})
	}
}
