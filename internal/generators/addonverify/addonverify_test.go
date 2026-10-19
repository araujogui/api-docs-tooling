package addonverify

import (
	"encoding/json"
	"strings"
	"testing"
)

func find(files []File, name string) *File {
	for i := range files {
		if files[i].Name == name {
			return &files[i]
		}
	}

	return nil
}

func TestGenerateFileList(t *testing.T) {
	t.Run("points test.js at the build tree", func(t *testing.T) {
		test := find(GenerateFileList([]File{{Name: "test.js", Content: "const addon = require('./build/Release/addon');"}}), "test.js")

		for _, want := range []string{"'use strict';", "require('../../common')", "`./build/${common.buildType}/addon`"} {
			if !strings.Contains(test.Content, want) {
				t.Errorf("missing %q in %q", want, test.Content)
			}
		}

		if strings.Contains(test.Content, "'./build/Release/addon'") {
			t.Error("kept the release path")
		}
	})

	t.Run("keeps other files", func(t *testing.T) {
		if got := find(GenerateFileList([]File{{Name: "addon.cc", Content: "#include <node.h>"}}), "addon.cc"); got.Content != "#include <node.h>" {
			t.Errorf("got %q", got.Content)
		}
	})

	t.Run("adds binding.gyp", func(t *testing.T) {
		binding := find(GenerateFileList([]File{{Name: "addon.cc", Content: "code"}}), "binding.gyp")

		var config struct {
			Targets []struct {
				TargetName string   `json:"target_name"`
				Sources    []string `json:"sources"`
			} `json:"targets"`
		}

		if err := json.Unmarshal([]byte(binding.Content), &config); err != nil {
			t.Fatal(err)
		}

		if config.Targets[0].TargetName != "addon" || config.Targets[0].Sources[0] != "addon.cc" {
			t.Errorf("got %+v", config)
		}
	})

	t.Run("handles no input", func(t *testing.T) {
		if got := GenerateFileList(nil); len(got) != 1 || got[0].Name != "binding.gyp" {
			t.Errorf("got %v", got)
		}
	})
}

func TestDedent(t *testing.T) {
	got := updateJSRequirePaths("const addon = require('./build/Release/addon');\nconsole.log('a\\nb');")
	want := "'use strict';\nconst common = require('../../common');\nconst addon = require(`./build/${common.buildType}/addon`);\nconsole.log('a\nb');"

	if got != want {
		t.Errorf("got %q", got)
	}
}

func TestIsBuildableSection(t *testing.T) {
	cases := []struct {
		files []File
		want  bool
	}{
		{[]File{{Name: "addon.cc"}, {Name: "test.js"}}, true},
		{[]File{{Name: "addon.cc"}}, false},
		{[]File{{Name: "test.js"}}, false},
		{nil, false},
	}

	for _, c := range cases {
		if got := IsBuildableSection(c.files); got != c.want {
			t.Errorf("IsBuildableSection(%v) = %v", c.files, got)
		}
	}
}

func TestNormalizeSectionName(t *testing.T) {
	cases := map[string]string{
		"Hello World":          "hello_world",
		"Test-Section!@#":      "testsection",
		"":                     "",
		"My Test & Example #1": "my_test__example_1",
	}

	for in, want := range cases {
		if got := NormalizeSectionName(in); got != want {
			t.Errorf("NormalizeSectionName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSectionFolderName(t *testing.T) {
	cases := []struct {
		name  string
		index int
		want  string
	}{
		{"hello_world", 0, "01_hello_world"},
		{"test", 5, "06_test"},
		{"example", 15, "16_example"},
		{"", 0, "01_"},
	}

	for _, c := range cases {
		if got := SectionFolderName(c.name, c.index); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}
