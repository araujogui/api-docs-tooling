package legacyhtml

import (
	"testing"

	"github.com/nodejs/doc-kit/internal/markdown"
	"github.com/nodejs/doc-kit/internal/unist"

	"github.com/nodejs/doc-kit/internal/parsers"
	"github.com/nodejs/doc-kit/internal/semver"
)

func TestVersionLabel(t *testing.T) {
	if got := VersionLabel(semver.Version{Major: 18}); got != "18.x" {
		t.Errorf("got %q", got)
	}

	if got := VersionLabel(semver.Version{Major: 18, Minor: 5, Patch: 2}); got != "18.5.x" {
		t.Errorf("got %q", got)
	}
}

func TestCompatibleVersions(t *testing.T) {
	releases := []parsers.Release{{Version: semver.Version{Major: 16}}, {Version: semver.Version{Major: 18}}, {Version: semver.Version{Major: 20}}}

	got := CompatibleVersions("18.0.0", releases)

	if len(got) != 2 || got[0].Version.Major != 18 || got[1].Version.Major != 20 {
		t.Errorf("got %v", got)
	}

	if got := CompatibleVersions("14.0.0", releases); len(got) != 3 {
		t.Errorf("got %v", got)
	}

	// An unknown version keeps every release
	if got := CompatibleVersions("invalid", releases); len(got) != 3 {
		t.Errorf("got %v", got)
	}
}

var markdownParse = func(md string) *unist.Node { return markdown.Parse(md, markdown.Options{GFM: true}) }
