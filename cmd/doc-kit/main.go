// Command doc-kit generates the Node.js API documentation.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nodejs/doc-kit/internal/generators"
	_ "github.com/nodejs/doc-kit/internal/generators/all"
	"github.com/nodejs/doc-kit/internal/logger"
	"github.com/spf13/cobra"
)

// jsGenerators are the generators that stay in JavaScript (they render
// React, or need TypeScript's compiler): runs requesting them hand those
// targets to the JavaScript CLI.
var jsGenerators = []string{"html", "web", "orama-db", "llms-txt", "llms-txt-full", "sitemap", "section-pages", "jsx-ast"}

func main() {
	var logLevel string

	root := &cobra.Command{
		Use:           "doc-kit",
		Short:         "CLI tool to generate the Node.js API documentation",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if _, ok := logger.Levels[logLevel]; !ok {
				return fmt.Errorf("invalid --log-level %q: expected one of %s", logLevel, strings.Join(logger.LevelNames, ", "))
			}

			logger.Default.SetLevel(logLevel)

			return nil
		},
	}

	root.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level ("+strings.Join(logger.LevelNames, ", ")+")")
	root.AddCommand(generateCommand())
	root.SetArgs(normalizeArgs(os.Args[1:]))

	if err := root.Execute(); err != nil {
		logger.Default.Error(err)
		os.Exit(1)
	}
}

func generateCommand() *cobra.Command {
	var opts generators.CLIOptions
	var jsCLI string

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate API docs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			user, err := generators.UserConfiguration(opts)
			if err != nil {
				return err
			}

			if err := generators.AssertRunnable(user); err != nil {
				return err
			}

			var native, delegated []string

			for _, t := range toStrings(user["target"]) {
				if _, ok := generators.Get(t); ok {
					native = append(native, t)
				} else {
					delegated = append(delegated, t)
				}
			}

			if len(delegated) > 0 {
				if err := runJavaScript(jsCLI, delegated); err != nil {
					return err
				}
			}

			if len(native) == 0 {
				return nil
			}

			cfg, err := generators.Configure(user, native)
			if err != nil {
				return err
			}

			_, err = generators.Run(cfg)

			return err
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.ConfigFile, "config-file", "", "Config file")
	f.StringArrayVarP(&opts.Input, "input", "i", nil, "Input file patterns (glob)")
	f.StringArrayVarP(&opts.Target, "target", "t", nil, "Target generator(s): "+strings.Join(append(generators.PublicNames(), jsGenerators...), ", "))
	f.StringArrayVar(&opts.Ignore, "ignore", nil, "Ignore file patterns (glob)")
	f.StringVarP(&opts.Output, "output", "o", "", "The output directory")
	f.StringVarP(&opts.Threads, "threads", "p", "", "Number of threads to use (minimum: 1)")
	f.StringVar(&opts.ChunkSize, "chunk-size", "", "Number of items to process per task (minimum: 1)")
	f.StringVarP(&opts.Version, "version", "v", "", "Target Node.js version")
	f.StringVarP(&opts.Changelog, "changelog", "c", "", "Changelog URL or path")
	f.StringVar(&opts.GitRef, "git-ref", "", "Git ref")
	f.StringVar(&opts.Index, "index", "", "index.md URL or path")
	f.BoolVar(&opts.Minify, "minify", false, "Minify?")
	f.StringVar(&opts.TypeMap, "type-map", "", "Type map URL or path")
	f.StringVar(&jsCLI, "js-cli", os.Getenv("DOC_KIT_JS_CLI"), "Path to the JavaScript CLI, which runs the generators that stay in JavaScript")

	return cmd
}

// variadicFlags take several values, as `-i a b` does in the JavaScript CLI.
var variadicFlags = map[string]bool{"-i": true, "--input": true, "-t": true, "--target": true, "--ignore": true}

// normalizeArgs repeats variadic flags for each of their values
// (`-i a b` becomes `-i a -i b`), which is how flags take several values here.
func normalizeArgs(args []string) []string {
	var out []string
	current := ""

	for _, arg := range args {
		switch {
		case variadicFlags[arg]:
			current = arg
			continue
		case strings.HasPrefix(arg, "-") && arg != "-":
			current = ""
			out = append(out, arg)
		case current != "":
			out = append(out, current, arg)
		default:
			out = append(out, arg)
		}
	}

	return out
}

func toStrings(v any) []string {
	switch x := v.(type) {
	case []any:
		out := make([]string, len(x))

		for i, item := range x {
			out[i] = fmt.Sprint(item)
		}

		return out
	case string:
		return []string{x}
	}

	return nil
}

// findJSCLI looks for the JavaScript CLI of this repository, from the
// working directory and the executable's.
func findJSCLI() string {
	var dirs []string

	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
	}

	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}

	for _, dir := range dirs {
		for d := dir; ; d = filepath.Dir(d) {
			candidate := filepath.Join(d, "packages", "cli", "bin", "cli.mjs")

			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}

			if filepath.Dir(d) == d {
				break
			}
		}
	}

	return ""
}

// runJavaScript runs targets with the JavaScript CLI, passing the rest of
// the command line through.
func runJavaScript(jsCLI string, targets []string) error {
	if jsCLI == "" {
		jsCLI = findJSCLI()
	}

	if jsCLI == "" {
		return fmt.Errorf("the %s generator(s) run in JavaScript: pass --js-cli (or set DOC_KIT_JS_CLI) to the path of @doc-kit/cli's bin/cli.mjs", strings.Join(targets, ", "))
	}

	args := []string{jsCLI}
	skip := false

	for _, arg := range normalizeArgs(os.Args[1:]) {
		if skip {
			skip = false
			continue
		}

		switch {
		case arg == "-t" || arg == "--target" || arg == "--js-cli":
			skip = true
			continue
		case strings.HasPrefix(arg, "--target=") || strings.HasPrefix(arg, "--js-cli="):
			continue
		}

		args = append(args, arg)
	}

	for _, t := range targets {
		args = append(args, "-t", t)
	}

	logger.Default.Child("generators").Debug("Running in JavaScript", logger.With("targets", targets))

	cmd := exec.Command("node", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
