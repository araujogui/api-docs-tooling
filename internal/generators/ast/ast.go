// Package ast is the generator parsing Markdown API doc files into mdast
// trees.
package ast

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/nodejs/doc-kit/internal/doc"
	"github.com/nodejs/doc-kit/internal/fsutil"
	"github.com/nodejs/doc-kit/internal/generators"
	"github.com/nodejs/doc-kit/internal/markdown"
	"github.com/nodejs/doc-kit/internal/queries"
	"github.com/nodejs/doc-kit/internal/unist"
	"gopkg.in/yaml.v3"
)

// StabilityIndexURL is where stability indexes link to.
const StabilityIndexURL = "documentation.html#stability-index"

func init() {
	generators.Register(&generators.Generator{
		Name:        "ast",
		Description: "Parses Markdown API doc files into AST trees",
		Generate:    generate,
	})
}

// isMDX reports whether a file is MDX: a `.mdx` extension opts in, but an
// `mdx` boolean in its frontmatter always takes precedence.
func isMDX(file, content string) bool {
	if m := queries.StandardYAMLFrontmatter.FindStringSubmatch(content); m != nil {
		var front map[string]any

		if yaml.Unmarshal([]byte(m[1]), &front) == nil {
			if mdx, ok := front["mdx"].(bool); ok {
				return mdx
			}
		}
	}

	return strings.HasSuffix(file, ".mdx")
}

// ParseFile parses a Markdown file; parent is the directory its path is
// relative to.
func ParseFile(file, parent string) (*doc.File, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	content := string(data)
	mdx := isMDX(file, content)

	// The (first) stability index becomes a link
	withStabilityLinks := content

	if loc := queries.StabilityIndexPrefix.FindStringIndex(content); loc != nil {
		match := content[loc[0]:loc[1]]
		withStabilityLinks = content[:loc[0]] + "[" + match + "](" + StabilityIndexURL + ")" + content[loc[1]:]
	}

	relative := "/" + fsutil.WithExt(fsutil.PosixRelative(parent, file), "")

	var tree *unist.Node

	frontmatter := queries.StandardYAMLFrontmatter.FindStringSubmatchIndex(withStabilityLinks)

	if mdx {
		// MDX can't hold the `<!-- -->` comment, so the frontmatter is
		// stripped and re-attached as the YAML `html` node the metadata
		// stage expects
		source := withStabilityLinks
		yamlBlock := ""

		if frontmatter != nil {
			yamlBlock = withStabilityLinks[frontmatter[2]:frontmatter[3]]
			source = withStabilityLinks[frontmatter[1]:]
		}

		tree = markdown.Parse(source, markdown.Options{GFM: true})

		if frontmatter != nil {
			tree.Children = append([]*unist.Node{{Type: "html", Value: "<!-- YAML\n" + yamlBlock + "\n-->"}}, tree.Children...)
		}
	} else {
		value := withStabilityLinks

		if frontmatter != nil {
			value = "<!-- YAML\n" + withStabilityLinks[frontmatter[2]:frontmatter[3]] + "\n-->" + withStabilityLinks[frontmatter[1]:]
		}

		tree = markdown.Parse(value, markdown.Options{GFM: true, TypeAnnotations: true})
	}

	return &doc.File{Tree: tree, Path: relative, MDX: mdx, Source: file}, nil
}

// globParent returns the directory a glob pattern's matches are under: its
// leading segments without glob characters.
func globParent(pattern string) string {
	pattern = filepath.ToSlash(pattern)

	if !strings.ContainsAny(pattern, "*?[]{}()!") {
		return path.Dir(pattern)
	}

	segments := strings.Split(pattern, "/")

	var kept []string

	for _, s := range segments {
		if strings.ContainsAny(s, "*?[]{}()!") {
			break
		}

		kept = append(kept, s)
	}

	parent := strings.Join(kept, "/")

	if parent == "" {
		if strings.HasPrefix(pattern, "/") {
			return "/"
		}

		return "."
	}

	return parent
}

func cleanPattern(p string) string {
	p = filepath.ToSlash(p)

	for strings.HasPrefix(p, "./") {
		p = p[2:]
	}

	return p
}

// Glob returns the files matching patterns, without those matching the
// ignore patterns, each with the directory its path is relative to.
func Glob(patterns, ignore []string) ([][2]string, error) {
	var files [][2]string

	for _, pattern := range patterns {
		matches, err := doublestar.FilepathGlob(cleanPattern(pattern), doublestar.WithFilesOnly())
		if err != nil {
			return nil, err
		}

		sort.Strings(matches)

		parent := globParent(cleanPattern(pattern))

	matching:
		for _, m := range matches {
			for _, ig := range ignore {
				if ok, _ := doublestar.PathMatch(cleanPattern(ig), filepath.ToSlash(m)); ok {
					continue matching
				}
			}

			files = append(files, [2]string{m, parent})
		}
	}

	return files, nil
}

func generate(ctx *generators.Context, _ any) (any, error) {
	files, err := Glob(ctx.Options.Strings("input"), ctx.Options.Strings("ignore"))
	if err != nil {
		return nil, err
	}

	parsed := make([]*doc.File, len(files))

	err = ctx.Run.Parallel(len(files), func(i int) error {
		f, err := ParseFile(files[i][0], files[i][1])
		parsed[i] = f

		return err
	})

	return parsed, err
}
