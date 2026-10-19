package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// searchPlaces are where a configuration file is looked for, in order, like
// cosmiconfig does for `doc-kit`.
var searchPlaces = []string{
	"package.json",
	".doc-kitrc",
	".doc-kitrc.json",
	".doc-kitrc.yaml",
	".doc-kitrc.yml",
	".doc-kitrc.js",
	".doc-kitrc.ts",
	".doc-kitrc.mjs",
	".doc-kitrc.cjs",
	".config/doc-kitrc",
	".config/doc-kitrc.json",
	".config/doc-kitrc.yaml",
	".config/doc-kitrc.yml",
	".config/doc-kitrc.js",
	".config/doc-kitrc.ts",
	".config/doc-kitrc.mjs",
	".config/doc-kitrc.cjs",
	"doc-kit.config.js",
	"doc-kit.config.ts",
	"doc-kit.config.mjs",
	"doc-kit.config.cjs",
}

// Presets are the configurations `extends` can name without a file.
var Presets = map[string]map[string]any{
	"@node-core/doc-kit/config": NodePreset(),
}

// NodePreset configures the generators the way nodejs.org builds its API
// documentation.
func NodePreset() map[string]any {
	repo := Options{"repository": "nodejs/node", "ref": "HEAD"}

	return map[string]any{
		"global": map[string]any{
			"project":    "Node.js",
			"repository": "nodejs/node",
			"ref":        "HEAD",
			"baseURL":    "https://nodejs.org/docs",
			"changelog":  Populate(ChangelogURL, repo),
		},
		"html": map[string]any{
			"editURL":         GitHubEditURL + "/doc/api{path}.md",
			"pageURL":         "{baseURL}/latest-{version}/api{path}.html",
			"remoteConfigUrl": "https://nodejs.org/site.json",
		},
		"llms-txt": map[string]any{
			"pageURL": "{baseURL}/latest/api{path}.md",
		},
		"sitemap": map[string]any{
			"indexURL": "{baseURL}/latest/api/",
			"pageURL":  "{baseURL}/latest/api{path}.html",
		},
	}
}

// LoadFile loads a configuration file (or the first one found in the working
// directory), with the presets it extends merged underneath it.
func LoadFile(path string) (map[string]any, string, error) {
	if path == "" {
		for _, place := range searchPlaces {
			if _, err := os.Stat(place); err != nil {
				continue
			}

			cfg, err := readFile(place)
			if err != nil {
				return nil, "", err
			}

			// package.json only counts with a `doc-kit` property
			if cfg == nil {
				continue
			}

			path = place

			return resolveExtends(cfg, path)
		}

		return map[string]any{}, "", nil
	}

	cfg, err := readFile(path)
	if err != nil {
		return nil, "", err
	}

	if cfg == nil {
		cfg = map[string]any{}
	}

	return resolveExtends(cfg, path)
}

func resolveExtends(cfg map[string]any, path string) (map[string]any, string, error) {
	presets := cfg["extends"]
	delete(cfg, "extends")

	var names []string

	switch v := presets.(type) {
	case string:
		names = []string{v}
	case []any:
		for _, item := range v {
			names = append(names, fmt.Sprint(item))
		}
	}

	// Later presets win over earlier ones; the file itself over all of them
	for i := len(names) - 1; i >= 0; i-- {
		preset, ok := Presets[names[i]]

		if !ok {
			file := names[i]

			if strings.HasPrefix(file, ".") {
				file = filepath.Join(filepath.Dir(path), file)
			}

			loaded, _, err := LoadFile(file)
			if err != nil {
				return nil, "", fmt.Errorf("loading preset %q: %w", names[i], err)
			}

			preset = loaded
		}

		cfg = DeepMerge(deepCopy(preset), cfg)
	}

	return cfg, path, nil
}

func readFile(path string) (map[string]any, error) {
	ext := filepath.Ext(path)
	base := filepath.Base(path)

	switch {
	case base == "package.json":
		var pkg map[string]any

		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(data, &pkg); err != nil {
			return nil, err
		}

		cfg, _ := pkg["doc-kit"].(map[string]any)

		return cfg, nil
	case ext == ".js" || ext == ".mjs" || ext == ".cjs" || ext == ".ts":
		return evaluateScript(path)
	default:
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		var cfg map[string]any

		if ext == ".json" {
			err = json.Unmarshal(data, &cfg)
		} else {
			err = yaml.Unmarshal(data, &cfg)
		}

		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}

		return normalize(cfg).(map[string]any), nil
	}
}

// evaluateScript loads a JavaScript configuration file with Node.js, which
// resolves the presets it extends too, and reads it back as JSON (dropping
// what JSON cannot hold).
func evaluateScript(path string) (map[string]any, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("node", "--input-type=module", "-e", evaluateConfigScript, abs)
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("evaluating %s with Node.js: %w", path, err)
	}

	var cfg map[string]any

	if err := json.Unmarshal(out, &cfg); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	return cfg, nil
}

// evaluateConfigScript imports a configuration module and the presets it
// extends, and prints the merged configuration as JSON.
const evaluateConfigScript = `
import { createRequire } from 'node:module';
import { dirname, isAbsolute, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const isPlainObject = v => v !== null && typeof v === 'object' && !Array.isArray(v);

const deepMerge = (...objects) => objects.reduce((result, source) => {
  for (const [key, value] of Object.entries(source)) {
    if (value === undefined) continue;
    result[key] = isPlainObject(value)
      ? deepMerge(isPlainObject(result[key]) ? result[key] : {}, value)
      : Array.isArray(value) ? value.slice() : value;
  }
  return result;
}, {});

const resolveSpecifier = (specifier, from) => {
  if (specifier.startsWith('file:')) return specifier;
  if (specifier.startsWith('.') || isAbsolute(specifier)) {
    return pathToFileURL(resolve(dirname(from), specifier)).href;
  }
  return pathToFileURL(createRequire(from).resolve(specifier)).href;
};

const file = process.argv.at(-1);
const module = await import(pathToFileURL(file).href);
let { extends: presets, ...config } = module.default ?? module;

for (const preset of [presets ?? []].flat().toReversed()) {
  const url = resolveSpecifier(preset, file);
  const imported = await import(url);
  config = deepMerge(imported.default ?? imported, config);
}

process.stdout.write(JSON.stringify(config));
`

// normalize turns YAML's decoded maps into string-keyed ones.
func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, item := range x {
			x[k] = normalize(item)
		}

		return x
	case map[any]any:
		m := make(map[string]any, len(x))

		for k, item := range x {
			m[fmt.Sprint(k)] = normalize(item)
		}

		return m
	case []any:
		for i, item := range x {
			x[i] = normalize(item)
		}

		return x
	case int:
		return float64(x)
	default:
		return v
	}
}

func deepCopy(v map[string]any) map[string]any {
	data, _ := json.Marshal(v)

	var out map[string]any

	_ = json.Unmarshal(data, &out)

	return out
}

// DeepMerge recursively merges plain objects from left to right into a new
// object: nested objects merge, arrays and other values replace, and nil
// values are skipped.
func DeepMerge(objects ...map[string]any) map[string]any {
	result := map[string]any{}

	for _, source := range objects {
		for key, value := range source {
			if value == nil {
				continue
			}

			if obj, ok := value.(map[string]any); ok {
				target, _ := result[key].(map[string]any)

				if target == nil {
					target = map[string]any{}
				}

				result[key] = DeepMerge(target, obj)
			} else if arr, ok := value.([]any); ok {
				result[key] = append([]any{}, arr...)
			} else {
				result[key] = value
			}
		}
	}

	return result
}
