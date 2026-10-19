package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDeepMerge(t *testing.T) {
	t.Run("later objects win", func(t *testing.T) {
		got := DeepMerge(map[string]any{"a": 1, "b": 2}, map[string]any{"b": 10, "c": 3})

		if !reflect.DeepEqual(got, map[string]any{"a": 1, "b": 10, "c": 3}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("nested objects merge", func(t *testing.T) {
		got := DeepMerge(map[string]any{"nested": map[string]any{"a": 1}}, map[string]any{"nested": map[string]any{"b": 2}})

		if !reflect.DeepEqual(got, map[string]any{"nested": map[string]any{"a": 1, "b": 2}}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("arrays replace, and nil values are skipped", func(t *testing.T) {
		got := DeepMerge(map[string]any{"a": []any{1}, "b": 1}, map[string]any{"a": []any{2}, "b": nil})

		if !reflect.DeepEqual(got, map[string]any{"a": []any{2}, "b": 1}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("the sources are left untouched", func(t *testing.T) {
		source := map[string]any{"nested": map[string]any{"a": 1}}
		DeepMerge(source, map[string]any{"nested": map[string]any{"b": 2}})

		if !reflect.DeepEqual(source, map[string]any{"nested": map[string]any{"a": 1}}) {
			t.Errorf("source changed: %v", source)
		}
	})
}

func write(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestLoadFile(t *testing.T) {
	t.Run("loads a JSON file", func(t *testing.T) {
		path := write(t, t.TempDir(), "doc-kit.json", `{"custom": "config"}`)
		got, _, err := LoadFile(path)

		if err != nil || got["custom"] != "config" {
			t.Errorf("got %v, %v", got, err)
		}
	})

	t.Run("loads a YAML file", func(t *testing.T) {
		path := write(t, t.TempDir(), ".doc-kitrc.yaml", "target: json\nglobal:\n  input: ['doc/*.md']\n  minify: true\n")
		got, _, err := LoadFile(path)

		if err != nil {
			t.Fatal(err)
		}

		global := got["global"].(map[string]any)

		if got["target"] != "json" || global["minify"] != true || !reflect.DeepEqual(global["input"], []any{"doc/*.md"}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("merges presets underneath the file", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "base.json", `{"global": {"project": "Base", "ref": "base"}, "html": {"a": 1}}`)
		write(t, dir, "other.json", `{"global": {"project": "Other"}, "html": {"b": 2}}`)
		path := write(t, dir, "doc-kit.config.json", `{"extends": ["./base.json", "./other.json"], "global": {"ref": "own"}}`)

		got, _, err := LoadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		// Later presets win over earlier ones; the file itself over all
		want := map[string]any{
			"global": map[string]any{"project": "Other", "ref": "own"},
			"html":   map[string]any{"a": float64(1), "b": float64(2)},
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("knows the Node.js preset", func(t *testing.T) {
		path := write(t, t.TempDir(), "doc-kit.json", `{"extends": "@node-core/doc-kit/config"}`)
		got, _, err := LoadFile(path)

		if err != nil {
			t.Fatal(err)
		}

		global := got["global"].(map[string]any)

		if global["project"] != "Node.js" || global["repository"] != "nodejs/node" {
			t.Errorf("got %v", global)
		}

		if global["changelog"] != "https://raw.githubusercontent.com/nodejs/node/HEAD/CHANGELOG.md" {
			t.Errorf("got %v", global["changelog"])
		}
	})

	t.Run("searches the working directory", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "package.json", `{"name": "x"}`)
		write(t, dir, ".doc-kitrc.json", `{"found": true}`)

		wd, _ := os.Getwd()
		os.Chdir(dir)
		defer os.Chdir(wd)

		got, path, err := LoadFile("")

		// package.json only counts with a `doc-kit` property
		if err != nil || got["found"] != true || path != ".doc-kitrc.json" {
			t.Errorf("got %v from %q, %v", got, path, err)
		}
	})

	t.Run("is empty without a file", func(t *testing.T) {
		wd, _ := os.Getwd()
		os.Chdir(t.TempDir())
		defer os.Chdir(wd)

		if got, _, err := LoadFile(""); err != nil || len(got) != 0 {
			t.Errorf("got %v, %v", got, err)
		}
	})
}
