package parsers

import (
	"reflect"
	"testing"

	"github.com/nodejs/doc-kit/internal/semver"
)

func TestParseChangelogContent(t *testing.T) {
	got := ParseChangelogContent("* [Node.js 24](doc/changelogs/CHANGELOG_V24.md) **Current**\n" +
		"* [Node.js 22](doc/changelogs/CHANGELOG_V22.md) **Long Term Support**\n")

	want := []Release{
		{Version: semver.Version{Major: 24}, IsLts: false},
		{Version: semver.Version{Major: 22}, IsLts: true},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestParseIndexContent(t *testing.T) {
	got := ParseIndexContent("# API Documentation\n* [Assert](assert.md)\n* [Buffer](buffer.md)\n" +
		"* [Child Process](child_process.md)\n* [Something](not-a-markdown-file)")

	want := []IndexEntry{
		{Section: "Assert", API: "assert"},
		{Section: "Buffer", API: "buffer"},
		{Section: "Child Process", API: "child_process"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}
