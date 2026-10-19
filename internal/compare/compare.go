// Package compare compares a generator's output with the base branch's, for
// the comment CI leaves on pull requests.
package compare

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

// Metadata files sit next to the output, but are not part of it.
const (
	BenchmarkFile  = "benchmark.json"
	ComparisonFile = "comparison.txt"
)

var units = []string{"B", "KB", "MB", "GB"}

// FormatBytes formats a byte count, like "1.50 KB".
func FormatBytes(bytes float64) string {
	if bytes == 0 {
		return "0 B"
	}

	unit := min(int(math.Floor(math.Log(math.Abs(bytes))/math.Log(1024))), len(units)-1)

	return fmt.Sprintf("%.2f %s", bytes/math.Pow(1024, float64(unit)), units[unit])
}

func formatSeconds(seconds float64) string {
	if math.Abs(seconds) < 1 {
		return fmt.Sprintf("%.2f ms", seconds*1000)
	}

	return fmt.Sprintf("%.2f s", seconds)
}

// ListOutputFiles lists a directory's files (relative, sorted), without the
// metadata files.
func ListOutputFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}

			return err
		}

		if d.IsDir() || d.Name() == BenchmarkFile || d.Name() == ComparisonFile {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		files = append(files, rel)

		return err
	})

	sort.Strings(files)

	return files, err
}

type benchmark struct {
	ElapsedSeconds *float64 `json:"elapsedSeconds"`
	MaxRSSKiB      *float64 `json:"maxRssKiB"`
}

func readBenchmark(dir string) (*benchmark, error) {
	data, err := os.ReadFile(filepath.Join(dir, BenchmarkFile))

	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	var b benchmark

	return &b, json.Unmarshal(data, &b)
}

func formatChange(base, head float64, increase, decrease string) string {
	diff := head - base

	if diff == 0 {
		return "unchanged"
	}

	percent := "n/a"

	if base != 0 {
		percent = fmt.Sprintf("%.1f%%", math.Abs(diff/base*100))
	}

	if diff > 0 {
		return percent + " " + increase
	}

	return percent + " " + decrease
}

// Performance compares the benchmarks of two outputs, if both have one.
func Performance(baseDir, headDir string) (string, error) {
	base, err := readBenchmark(baseDir)
	if err != nil {
		return "", err
	}

	head, err := readBenchmark(headDir)
	if err != nil {
		return "", err
	}

	if base == nil || head == nil {
		return "", nil
	}

	metrics := []struct {
		label              string
		base, head         *float64
		scale              float64
		format             func(float64) string
		increase, decrease string
	}{
		{"Generation time", base.ElapsedSeconds, head.ElapsedSeconds, 1, formatSeconds, "slower", "faster"},
		{"Peak memory", base.MaxRSSKiB, head.MaxRSSKiB, 1024, FormatBytes, "higher", "lower"},
	}

	rows := []string{"**Performance estimate** <sub>(single CI run)</sub>"}

	for _, m := range metrics {
		if m.base == nil || m.head == nil {
			return "", fmt.Errorf("Invalid %s benchmark value", strings.ToLower(m.label))
		}

		b, h := *m.base*m.scale, *m.head*m.scale
		rows = append(rows, fmt.Sprintf("- **%s:** %s (%s → %s)", m.label, formatChange(b, h, m.increase, m.decrease), m.format(b), m.format(h)))
	}

	return strings.Join(rows, "\n"), nil
}

func sizes(dir string) (map[string]int64, error) {
	files, err := ListOutputFiles(dir)
	if err != nil {
		return nil, err
	}

	out := map[string]int64{}

	for _, f := range files {
		info, err := os.Stat(filepath.Join(dir, f))
		if err != nil {
			return nil, err
		}

		out[f] = info.Size()
	}

	return out, nil
}

func sign(n float64) string {
	if n > 0 {
		return "+"
	}

	return ""
}

// FileSize reports the output files whose presence or size changed.
func FileSize(baseDir, headDir string) (string, error) {
	base, err := sizes(baseDir)
	if err != nil {
		return "", err
	}

	head, err := sizes(headDir)
	if err != nil {
		return "", err
	}

	type change struct {
		file       string
		base, head int64
	}

	var changed []change

	seen := map[string]bool{}

	for _, m := range []map[string]int64{base, head} {
		for f := range m {
			_, inBase := base[f]
			_, inHead := head[f]

			if !seen[f] && (base[f] != head[f] || inBase != inHead) {
				changed = append(changed, change{f, base[f], head[f]})
			}

			seen[f] = true
		}
	}

	sort.SliceStable(changed, func(i, j int) bool {
		di, dj := math.Abs(float64(changed[i].head-changed[i].base)), math.Abs(float64(changed[j].head-changed[j].base))

		if di != dj {
			return di > dj
		}

		return changed[i].file < changed[j].file
	})

	var sections []string

	if len(changed) > 0 {
		var total int64
		var rows []string

		for _, c := range changed {
			diff := float64(c.head - c.base)
			total += c.head - c.base

			percent := ""

			if c.base != 0 {
				percent = fmt.Sprintf(" (%s%.1f%%)", sign(diff), diff/float64(c.base)*100)
			}

			baseSize, headSize := "—", "—"

			if _, ok := base[c.file]; ok {
				baseSize = FormatBytes(float64(c.base))
			}

			if _, ok := head[c.file]; ok {
				headSize = FormatBytes(float64(c.head))
			}

			rows = append(rows, fmt.Sprintf("| `%s` | %s | %s | %s%s%s |", c.file, baseSize, headSize, sign(diff), FormatBytes(diff), percent))
		}

		noun := "files"

		if len(changed) == 1 {
			noun = "file"
		}

		sections = append(sections, strings.Join([]string{
			fmt.Sprintf("**Output size:** %d %s changed · net %s%s", len(changed), noun, sign(float64(total)), FormatBytes(float64(total))),
			"",
			"<details>",
			"<summary>File size details</summary>",
			"",
			"| File | Main | PR | Change |",
			"| --- | ---: | ---: | ---: |",
			strings.Join(rows, "\n"),
			"",
			"</details>",
		}, "\n"))
	}

	return withPerformance(sections, baseDir, headDir)
}

func withPerformance(sections []string, baseDir, headDir string) (string, error) {
	perf, err := Performance(baseDir, headDir)
	if err != nil {
		return "", err
	}

	if perf != "" {
		sections = append(sections, perf)
	}

	return strings.Join(sections, "\n\n"), nil
}

func readJSON(path string) (any, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	var v any

	if err := json.Unmarshal(data, &v); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}

	pretty, _ := json.MarshalIndent(v, "", "  ")

	return v, pretty, nil
}

// diffLines is a unified-style diff of the lines that differ between two
// texts, around their common start and end.
func diffLines(a, b string) string {
	x, y := strings.Split(a, "\n"), strings.Split(b, "\n")
	start := 0

	for start < len(x) && start < len(y) && x[start] == y[start] {
		start++
	}

	endX, endY := len(x), len(y)

	for endX > start && endY > start && x[endX-1] == y[endY-1] {
		endX--
		endY--
	}

	const limit = 50

	var out []string

	for i, line := range x[start:endX] {
		if i == limit {
			out = append(out, "...")
			break
		}

		out = append(out, "- "+line)
	}

	for i, line := range y[start:endY] {
		if i == limit {
			out = append(out, "...")
			break
		}

		out = append(out, "+ "+line)
	}

	return strings.Join(out, "\n")
}

// ObjectAssertion reports the JSON output files whose contents changed.
func ObjectAssertion(baseDir, headDir string) (string, error) {
	baseFiles, err := ListOutputFiles(baseDir)
	if err != nil {
		return "", err
	}

	headFiles, err := ListOutputFiles(headDir)
	if err != nil {
		return "", err
	}

	inBase, inHead := map[string]bool{}, map[string]bool{}

	for _, f := range baseFiles {
		inBase[f] = true
	}

	for _, f := range headFiles {
		inHead[f] = true
	}

	all := append([]string{}, baseFiles...)

	for _, f := range headFiles {
		if !inBase[f] {
			all = append(all, f)
		}
	}

	var results []string

	for _, f := range all {
		switch {
		case !inBase[f]:
			results = append(results, fmt.Sprintf("- `%s` added", f))
		case !inHead[f]:
			results = append(results, fmt.Sprintf("- `%s` removed", f))
		default:
			b, bText, err := readJSON(filepath.Join(baseDir, f))
			if err != nil {
				return "", err
			}

			h, hText, err := readJSON(filepath.Join(headDir, f))
			if err != nil {
				return "", err
			}

			if !reflect.DeepEqual(b, h) {
				results = append(results, fmt.Sprintf("<details>\n<summary>%s</summary>\n\n```diff\n%s\n```\n\n</details>", f, diffLines(string(bText), string(hText))))
			}
		}
	}

	var sections []string

	if len(results) > 0 {
		noun := "files differ"

		if len(results) == 1 {
			noun = "file differs"
		}

		sections = append(sections, fmt.Sprintf("**Output:** %d %s", len(results), noun), strings.Join(results, "\n"))
	}

	return withPerformance(sections, baseDir, headDir)
}
