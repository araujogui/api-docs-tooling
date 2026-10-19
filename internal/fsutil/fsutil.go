// Package fsutil holds the file helpers the generators share.
package fsutil

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nodejs/doc-kit/internal/omap"
)

var extension = regexp.MustCompile(`(?i)\.[0-9a-z]+$`)

// WithExt replaces a path's extension (or removes it, when ext is empty).
func WithExt(path, ext string) string {
	path = extension.ReplaceAllString(path, "")

	if ext == "" {
		return path
	}

	return path + "." + ext
}

// WriteFile writes a file, creating its directory.
func WriteFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

// WriteJSON writes a value as JSON, indented by two spaces unless minified.
func WriteJSON(path string, value any, minify bool) error {
	indent := 2

	if minify {
		indent = 0
	}

	data, err := omap.Stringify(value, indent)
	if err != nil {
		return err
	}

	return WriteFile(path, data)
}

// LoadFromURL reads a file, or fetches a URL.
func LoadFromURL(location string) (string, error) {
	u, err := url.Parse(location)

	if err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		resp, err := http.Get(location)
		if err != nil {
			return "", err
		}

		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return "", fmt.Errorf("fetching %s: %s", location, resp.Status)
		}

		body, err := io.ReadAll(resp.Body)

		return string(body), err
	}

	if err == nil && u.Scheme == "file" {
		location = u.Path
	}

	data, err := os.ReadFile(location)

	return string(data), err
}

// CopyDir copies a directory's contents into another, recursively.
func CopyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return WriteFile(target, data)
	})
}

// PosixRelative returns the relative path from a directory to a path, with
// forward slashes.
func PosixRelative(from, to string) string {
	rel, err := filepath.Rel(from, to)

	if err != nil {
		return to
	}

	return strings.ReplaceAll(rel, string(filepath.Separator), "/")
}
