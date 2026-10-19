package generators

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/nodejs/doc-kit/internal/config"
	"github.com/nodejs/doc-kit/internal/logger"
	"github.com/nodejs/doc-kit/internal/parsers"
	"github.com/nodejs/doc-kit/internal/semver"
)

// DefaultMaxThreads caps the default number of threads.
const DefaultMaxThreads = 4

// DefaultChunkSize is the default number of items each task processes.
const DefaultChunkSize = 10

// Configuration is a run's configuration.
type Configuration struct {
	// Target lists the generators to run.
	Target []string
	// Threads is how many items are processed at once.
	Threads int
	// ChunkSize is how many items each task processes.
	ChunkSize int
	// Global holds the settings every generator inherits.
	Global config.Options

	generators map[string]config.Options
}

// Options returns a generator's settings, with the global ones it inherits.
func (c *Configuration) Options(name string) config.Options {
	if o, ok := c.generators[name]; ok {
		return o
	}

	return c.Global
}

// Parallel calls fn for each index below n, running up to Threads calls at
// once, and returns the first error.
func (c *Configuration) Parallel(n int, fn func(i int) error) error {
	threads := 1

	if c != nil && c.Threads > 1 {
		threads = c.Threads
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
		next     int
	)

	worker := func() {
		defer wg.Done()

		for {
			mu.Lock()

			if next >= n || firstErr != nil {
				mu.Unlock()

				return
			}

			i := next
			next++
			mu.Unlock()

			if err := fn(i); err != nil {
				mu.Lock()

				if firstErr == nil {
					firstErr = err
				}

				mu.Unlock()
			}
		}
	}

	for range min(threads, max(n, 1)) {
		wg.Add(1)

		go worker()
	}

	wg.Wait()

	return firstErr
}

// CLIOptions are the `generate` command's flags (empty when not given).
type CLIOptions struct {
	ConfigFile string
	Input      []string
	Target     []string
	Ignore     []string
	Output     string
	Threads    string
	ChunkSize  string
	Version    string
	Changelog  string
	GitRef     string
	Index      string
	Minify     bool
	TypeMap    string
}

func (o CLIOptions) toConfig() map[string]any {
	global := map[string]any{}
	set := func(m map[string]any, key string, value any) {
		switch v := value.(type) {
		case string:
			if v != "" {
				m[key] = v
			}
		case []string:
			if len(v) > 0 {
				items := make([]any, len(v))

				for i, s := range v {
					items[i] = s
				}

				m[key] = items
			}
		case bool:
			if v {
				m[key] = true
			}
		}
	}

	set(global, "input", o.Input)
	set(global, "ignore", o.Ignore)
	set(global, "output", o.Output)
	set(global, "minify", o.Minify)
	set(global, "ref", o.GitRef)
	set(global, "version", o.Version)
	set(global, "changelog", o.Changelog)
	set(global, "index", o.Index)

	cfg := map[string]any{"global": global}

	if o.TypeMap != "" {
		cfg["metadata"] = map[string]any{"typeMap": o.TypeMap}
	}

	set(cfg, "target", o.Target)

	if n, err := strconv.Atoi(o.Threads); err == nil {
		cfg["threads"] = float64(n)
	}

	if n, err := strconv.Atoi(o.ChunkSize); err == nil {
		cfg["chunkSize"] = float64(n)
	}

	return cfg
}

// detectProject reads the documented project's name from the working
// directory's package.json.
func detectProject() string {
	data, err := os.ReadFile("package.json")

	if err != nil {
		return ""
	}

	var pkg struct {
		Name string `json:"name"`
	}

	if json.Unmarshal(data, &pkg) != nil {
		return ""
	}

	return pkg.Name
}

// defaultVersion is the version documented when none is given: that of the
// Node.js on the PATH, as the original tooling used its own runtime's.
func defaultVersion() string {
	out, err := exec.Command("node", "--version").Output()

	if err != nil {
		return "v0.0.0"
	}

	return strings.TrimSpace(string(out))
}

func defaultThreads() int {
	if runtime.GOARCH == "riscv64" {
		return 1
	}

	return min(runtime.NumCPU(), DefaultMaxThreads)
}

// JSOnlyTargets returns the requested targets this binary has no generator
// for.
func JSOnlyTargets(targets []string) []string {
	var missing []string

	for _, t := range targets {
		if _, ok := Get(t); !ok {
			missing = append(missing, t)
		}
	}

	return missing
}

// UserConfiguration merges the configuration file and the CLI options, the
// latter winning.
func UserConfiguration(opts CLIOptions) (map[string]any, error) {
	file, _, err := config.LoadFile(opts.ConfigFile)
	if err != nil {
		return nil, err
	}

	if t, ok := file["target"].(string); ok {
		file["target"] = []any{t}
	}

	return config.DeepMerge(file, opts.toConfig()), nil
}

// Configure assembles a run's configuration: the defaults, then the
// configuration file, then the CLI options. Generators inherit the global
// settings they don't set themselves.
func Configure(user map[string]any, targets []string) (*Configuration, error) {
	generators, err := Load(targets)
	if err != nil {
		return nil, err
	}

	project := detectProject()

	if project == "" {
		project = "API Docs"
	}

	defaults := map[string]any{
		"global": map[string]any{
			"project":     project,
			"version":     defaultVersion(),
			"minify":      false,
			"ref":         "HEAD",
			"changelog":   []any{},
			"pathsToCopy": []any{"assets", "public", "static"},
		},
		"threads":   float64(defaultThreads()),
		"chunkSize": float64(DefaultChunkSize),
	}

	for name, g := range generators {
		if g.Defaults != nil {
			defaults[name] = g.Defaults(user)
		} else {
			defaults[name] = map[string]any{}
		}
	}

	merged := config.DeepMerge(defaults, user)

	cfg := &Configuration{
		Target:     targets,
		Threads:    max(toInt(merged["threads"]), 1),
		ChunkSize:  max(toInt(merged["chunkSize"]), 1),
		generators: map[string]config.Options{},
	}

	if runtime.GOARCH == "riscv64" && cfg.Threads > 1 {
		logger.Default.Warn(fmt.Sprintf("Using %d threads might cause failures on riscv64 with sv39. Please consider using only a single thread.", cfg.Threads))
	}

	global, _ := merged["global"].(map[string]any)

	cfg.Global, err = transform(config.Options(global))
	if err != nil {
		return nil, err
	}

	for name := range generators {
		own, _ := merged[name].(map[string]any)

		options, err := transform(config.Options(own))
		if err != nil {
			return nil, err
		}

		if md, ok := own["markdown"].(map[string]any); ok && len(md) > 0 {
			logger.Default.Warn(fmt.Sprintf("Ignoring `%s.markdown`: Markdown plugins are not supported by the Go generators.", name))
		}

		// Inherit the global settings it doesn't set
		for k, v := range cfg.Global {
			if _, ok := options[k]; !ok {
				options[k] = v
			}
		}

		cfg.generators[name] = options
	}

	return cfg, nil
}

// AssertRunnable checks a configuration has targets and input.
func AssertRunnable(user map[string]any) error {
	global, _ := user["global"].(map[string]any)

	if user["target"] == nil || global["input"] == nil {
		return errors.New("Both a `target` and an `input` must be provided, either via " +
			"`--target`/`--input` or a configuration file. " +
			"Run `doc-kit generate --help` for usage.")
	}

	return nil
}

func toInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case string:
		n, _ := strconv.Atoi(x)

		return n
	}

	return 0
}

// transform parses the settings that need it: the version is coerced, and the
// changelog and index (paths or URLs) are read.
func transform(o config.Options) (config.Options, error) {
	if o == nil {
		o = config.Options{}
	} else {
		o = o.Clone()
	}

	if v, ok := o["version"].(string); ok {
		if version := semver.Coerce(v); version != nil {
			o["version"] = *version
		} else {
			o["version"] = nil
		}
	}

	switch v := o["changelog"].(type) {
	case string:
		releases, err := parsers.ParseChangelog(v)
		if err != nil {
			return nil, fmt.Errorf("reading the changelog: %w", err)
		}

		o["changelog"] = releases
	case []any:
		releases := []parsers.Release{}

		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				releases = append(releases, parsers.Release{
					Version: semver.MustCoerce(config.Options(m).String("version")),
					IsLts:   config.Options(m).Bool("isLts"),
				})
			}
		}

		o["changelog"] = releases
	}

	switch v := o["index"].(type) {
	case string:
		index, err := parsers.ParseIndex(v)
		if err != nil {
			return nil, fmt.Errorf("reading the index: %w", err)
		}

		o["index"] = index
	case []any:
		var index []parsers.IndexEntry

		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				index = append(index, parsers.IndexEntry{Section: config.Options(m).String("section"), API: config.Options(m).String("api")})
			}
		}

		o["index"] = index
	}

	return o, nil
}
