// Command compare compares a generator's output (HEAD, default `out`) with
// the base branch's (BASE, default `base`), printing a Markdown summary for
// the pull request comment: `compare file-size` or `compare object-assertion`.
package main

import (
	"fmt"
	"os"

	"github.com/nodejs/doc-kit/internal/compare"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}

func main() {
	comparators := map[string]func(base, head string) (string, error){
		"file-size":        compare.FileSize,
		"object-assertion": compare.ObjectAssertion,
	}

	if len(os.Args) != 2 || comparators[os.Args[1]] == nil {
		fmt.Fprintln(os.Stderr, "usage: compare file-size|object-assertion")
		os.Exit(2)
	}

	result, err := comparators[os.Args[1]](env("BASE", "base"), env("HEAD", "out"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	title := env("TITLE", "## `"+env("GENERATOR", "...")+"` Generator")

	if result != "" {
		fmt.Printf("%s\n\n%s\n\n", title, result)
	}
}
