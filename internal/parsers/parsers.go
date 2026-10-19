// Package parsers reads the release history and the documentation index.
package parsers

import (
	"regexp"

	"github.com/nodejs/doc-kit/internal/fsutil"
	"github.com/nodejs/doc-kit/internal/semver"
)

// Release is a Node.js major version and its LTS status.
type Release struct {
	Version   semver.Version `json:"version"`
	IsLts     bool           `json:"isLts"`
	IsCurrent bool           `json:"isCurrent,omitempty"`
}

// IndexEntry is a document listed in the index.
type IndexEntry struct {
	Section string `json:"section"`
	API     string `json:"api"`
}

var (
	// The Node.js version headers of a CHANGELOG.md
	nodeVersions = regexp.MustCompile(`\* \[Node\.js ([0-9.]+)\]\S+ (.*)\r?\n`)
	// The list items of the index document
	listItem = regexp.MustCompile(`\* \[(.*?)\]\((.*?)\.md\)`)
	// Whether a Node.js version is an LTS release
	ltsVersion = regexp.MustCompile(`(?i)Long Term Support`)
)

// ParseChangelogContent parses the Node.js major versions out of a
// CHANGELOG.md's content.
func ParseChangelogContent(changelog string) []Release {
	releases := []Release{}

	for _, m := range nodeVersions.FindAllStringSubmatch(changelog, -1) {
		releases = append(releases, Release{
			Version: semver.MustCoerce(m[1]),
			IsLts:   ltsVersion.MatchString(m[2]),
		})
	}

	return releases
}

// ParseChangelog reads the Node.js major versions from a CHANGELOG.md (a path
// or URL).
func ParseChangelog(location string) ([]Release, error) {
	changelog, err := fsutil.LoadFromURL(location)
	if err != nil {
		return nil, err
	}

	return ParseChangelogContent(changelog), nil
}

// ParseIndexContent parses the documents listed in an index document.
func ParseIndexContent(index string) []IndexEntry {
	entries := []IndexEntry{}

	for _, m := range listItem.FindAllStringSubmatch(index, -1) {
		entries = append(entries, IndexEntry{Section: m[1], API: m[2]})
	}

	return entries
}

// ParseIndex reads the documents listed in an index document (a path or
// URL).
func ParseIndex(location string) ([]IndexEntry, error) {
	if location == "" {
		return []IndexEntry{}, nil
	}

	index, err := fsutil.LoadFromURL(location)
	if err != nil {
		return nil, err
	}

	return ParseIndexContent(index), nil
}
