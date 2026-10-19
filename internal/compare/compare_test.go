package compare

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func dirs(t *testing.T) (string, string) {
	t.Helper()

	root := t.TempDir()
	base, head := filepath.Join(root, "base"), filepath.Join(root, "head")
	os.MkdirAll(base, 0o755)
	os.MkdirAll(head, 0o755)

	return base, head
}

func write(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const sampleBenchmark = `{"elapsedSeconds": 2, "userCpuSeconds": 1, "systemCpuSeconds": 0.5, "maxRssKiB": 1024}`

func mustMatch(t *testing.T, s, pattern string) {
	t.Helper()

	if !regexp.MustCompile(pattern).MatchString(s) {
		t.Errorf("%q does not match %s", s, pattern)
	}
}

func TestPerformance(t *testing.T) {
	t.Run("summarizes benchmark differences", func(t *testing.T) {
		base, head := dirs(t)
		write(t, filepath.Join(base, BenchmarkFile), sampleBenchmark)
		write(t, filepath.Join(head, BenchmarkFile), `{"elapsedSeconds": 3, "userCpuSeconds": 0.75, "systemCpuSeconds": 0.5, "maxRssKiB": 1536}`)

		got, err := Performance(base, head)
		if err != nil {
			t.Fatal(err)
		}

		mustMatch(t, got, `\*\*Generation time:\*\* 50\.0% slower \(2\.00 s → 3\.00 s\)`)
		mustMatch(t, got, `\*\*Peak memory:\*\* 50\.0% higher \(1\.00 MB → 1\.50 MB\)`)
		mustMatch(t, got, `single CI run`)
	})

	t.Run("is empty when an output has no benchmark", func(t *testing.T) {
		base, head := dirs(t)
		write(t, filepath.Join(head, BenchmarkFile), sampleBenchmark)

		if got, err := Performance(base, head); got != "" || err != nil {
			t.Errorf("got %q, %v", got, err)
		}
	})

	t.Run("rejects invalid values", func(t *testing.T) {
		base, head := dirs(t)
		write(t, filepath.Join(base, BenchmarkFile), sampleBenchmark)
		write(t, filepath.Join(head, BenchmarkFile), `{"elapsedSeconds": 2}`)

		if _, err := Performance(base, head); err == nil || !strings.Contains(err.Error(), "Invalid peak memory benchmark value") {
			t.Errorf("got %v", err)
		}
	})
}

func TestFileSize(t *testing.T) {
	base, head := dirs(t)
	write(t, filepath.Join(base, "result.txt"), "base")
	write(t, filepath.Join(head, "result.txt"), "a larger result")
	write(t, filepath.Join(base, BenchmarkFile), sampleBenchmark)
	write(t, filepath.Join(head, BenchmarkFile), `{"elapsedSeconds": 3, "maxRssKiB": 1024}`)

	got, err := FileSize(base, head)
	if err != nil {
		t.Fatal(err)
	}

	mustMatch(t, got, `Output size:.*1 file changed · net \+11\.00 B`)
	mustMatch(t, got, `<summary>File size details</summary>`)
	mustMatch(t, got, `\| File \| Main \| PR \| Change \|\n\| --- \| ---: \| ---: \| ---: \|`)
	mustMatch(t, got, "`result\\.txt`")
	mustMatch(t, got, `Performance estimate`)

	if strings.Contains(got, BenchmarkFile) {
		t.Error("the benchmark is not output")
	}
}

func TestObjectAssertion(t *testing.T) {
	t.Run("treats the benchmark as metadata", func(t *testing.T) {
		base, head := dirs(t)
		write(t, filepath.Join(base, "result.json"), `{"value":true}`)
		write(t, filepath.Join(head, "result.json"), `{"value": true}`)
		write(t, filepath.Join(base, BenchmarkFile), sampleBenchmark)
		write(t, filepath.Join(head, BenchmarkFile), `{"elapsedSeconds": 2, "maxRssKiB": 2048}`)

		got, err := ObjectAssertion(base, head)
		if err != nil {
			t.Fatal(err)
		}

		if strings.Contains(got, "**Output:") || strings.Contains(got, BenchmarkFile) || !strings.Contains(got, "Performance estimate") {
			t.Errorf("got %q", got)
		}
	})

	t.Run("shows what changed", func(t *testing.T) {
		base, head := dirs(t)
		write(t, filepath.Join(base, "result.json"), `{"a": 1, "b": 2}`)
		write(t, filepath.Join(head, "result.json"), `{"a": 1, "b": 3}`)

		got, _ := ObjectAssertion(base, head)

		mustMatch(t, got, `\*\*Output:\*\* 1 file differs`)
		mustMatch(t, got, `- +"b": 2\n\+ +"b": 3`)
	})
}

func TestAddedAndRemoved(t *testing.T) {
	base, head := dirs(t)
	write(t, filepath.Join(base, "generator", "removed.json"), `{"old":true}`)
	write(t, filepath.Join(head, "generator", "added.json"), `{"new":true}`)
	write(t, filepath.Join(head, ComparisonFile), "")

	sizes, _ := FileSize(base, head)
	objects, _ := ObjectAssertion(base, head)

	added := regexp.QuoteMeta(filepath.Join("generator", "added.json"))
	removed := regexp.QuoteMeta(filepath.Join("generator", "removed.json"))

	mustMatch(t, sizes, `2 files changed`)
	mustMatch(t, sizes, "`"+added+"` \\| — \\| 12\\.00 B")
	mustMatch(t, sizes, "`"+removed+"` \\| 12\\.00 B \\| —")
	mustMatch(t, objects, "`"+added+"` added")
	mustMatch(t, objects, "`"+removed+"` removed")

	if strings.Contains(sizes, ComparisonFile) || strings.Contains(objects, ComparisonFile) {
		t.Error("comparison.txt is not output")
	}
}
