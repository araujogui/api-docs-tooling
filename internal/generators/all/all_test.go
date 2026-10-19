package all

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nodejs/doc-kit/internal/generators"
	"github.com/nodejs/doc-kit/internal/semver"
)

var names = []string{
	"ast", "ast-js", "metadata", "json", "json-all", "json-simple", "legacy-html", "legacy-html-all",
	"legacy-json", "legacy-json-all", "man-page", "addon-verify", "api-links",
}

func TestRegistry(t *testing.T) {
	for _, name := range names {
		g, ok := generators.Get(name)

		if !ok {
			t.Errorf("%q is not registered", name)
			continue
		}

		if g.Name != name {
			t.Errorf("%q is registered as %q", name, g.Name)
		}

		if g.DependsOn != "" {
			if _, ok := generators.Get(g.DependsOn); !ok {
				t.Errorf("%q depends on unknown %q", name, g.DependsOn)
			}
		}
	}

	if ast, _ := generators.Get("ast"); ast.DependsOn != "" {
		t.Error("ast should have no dependency")
	}

	// The parsers are internal
	for _, name := range generators.PublicNames() {
		if name == "ast" || name == "ast-js" || name == "metadata" {
			t.Errorf("%q should not be public", name)
		}
	}
}

func TestJSOnlyTargets(t *testing.T) {
	got := generators.JSOnlyTargets([]string{"json", "html", "legacy-html", "orama-db"})

	if len(got) != 2 || got[0] != "html" || got[1] != "orama-db" {
		t.Errorf("got %v", got)
	}
}

func configure(t *testing.T, opts generators.CLIOptions, file string) *generators.Configuration {
	t.Helper()

	if file != "" {
		path := filepath.Join(t.TempDir(), "doc-kit.json")
		os.WriteFile(path, []byte(file), 0o644)
		opts.ConfigFile = path
	}

	user, err := generators.UserConfiguration(opts)
	if err != nil {
		t.Fatal(err)
	}

	var targets []string

	for _, v := range user["target"].([]any) {
		targets = append(targets, v.(string))
	}

	cfg, err := generators.Configure(user, targets)
	if err != nil {
		t.Fatal(err)
	}

	return cfg
}

func TestConfigure(t *testing.T) {
	t.Run("CLI options override the configuration file", func(t *testing.T) {
		cfg := configure(t, generators.CLIOptions{Output: "custom-dist/", Version: "20.0.0", Target: []string{"json", "man-page"}, Threads: "2"},
			`{"global": {"input": "custom-src/", "output": "config-dist/", "version": "18.0.0"}, "target": ["json"], "threads": 1}`)

		o := cfg.Options("json")

		if o.String("input") != "custom-src/" || o.String("output") != "custom-dist/" || o.Version() != (semver.Version{Major: 20}) {
			t.Errorf("got %v", o)
		}

		if len(cfg.Target) != 2 || cfg.Threads != 2 {
			t.Errorf("got %v, %d threads", cfg.Target, cfg.Threads)
		}
	})

	t.Run("enforces minimums", func(t *testing.T) {
		cfg := configure(t, generators.CLIOptions{Target: []string{"json"}, Threads: "-5", ChunkSize: "0"}, "")

		if cfg.Threads != 1 || cfg.ChunkSize != 1 {
			t.Errorf("got %d threads, chunks of %d", cfg.Threads, cfg.ChunkSize)
		}
	})

	t.Run("defaults to project-neutral values", func(t *testing.T) {
		o := configure(t, generators.CLIOptions{Target: []string{"json"}}, "").Options("json")

		if o["repository"] != nil || o["baseURL"] != nil || len(o.Changelog()) != 0 || o.String("project") == "" {
			t.Errorf("got %v", o)
		}
	})

	t.Run("gives generators their defaults, under their own settings", func(t *testing.T) {
		cfg := configure(t, generators.CLIOptions{Target: []string{"legacy-json", "man-page"}},
			`{"global": {"minify": true, "ref": "v20"}, "man-page": {"fileName": "custom.1"}}`)

		// legacy-json defaults its own `minify` and `ref`, which win over the global ones
		if lj := cfg.Options("legacy-json"); lj.Bool("minify") || lj.String("ref") != "main" || lj.String("sourceURL") != "doc/api/{path}" {
			t.Errorf("got %v", lj)
		}

		if mp := cfg.Options("man-page"); mp.String("fileName") != "custom.1" || mp.String("ref") != "v20" || !mp.Bool("minify") {
			t.Errorf("got %v", mp)
		}
	})

	t.Run("parses the changelog and index", func(t *testing.T) {
		dir := t.TempDir()
		changelog := filepath.Join(dir, "CHANGELOG.md")
		index := filepath.Join(dir, "index.md")
		os.WriteFile(changelog, []byte("* [Node.js 24](x) **Current**\n* [Node.js 22](x) **Long Term Support**\n"), 0o644)
		os.WriteFile(index, []byte("* [Assert](assert.md)\n"), 0o644)

		o := configure(t, generators.CLIOptions{Target: []string{"json"}, Changelog: changelog, Index: index}, "").Options("json")

		if len(o.Changelog()) != 2 || !o.Changelog()[1].IsLts || len(o.Index()) != 1 || o.Index()[0].API != "assert" {
			t.Errorf("got %v, %v", o.Changelog(), o.Index())
		}
	})
}

func TestAssertRunnable(t *testing.T) {
	cases := []struct {
		cfg map[string]any
		ok  bool
	}{
		{map[string]any{"global": map[string]any{"input": "src/"}}, false},
		{map[string]any{"target": []any{"json"}, "global": map[string]any{}}, false},
		{map[string]any{"target": []any{"json"}, "global": map[string]any{"input": "src/"}}, true},
	}

	for _, c := range cases {
		if err := generators.AssertRunnable(c.cfg); (err == nil) != c.ok {
			t.Errorf("AssertRunnable(%v) = %v", c.cfg, err)
		}
	}
}
