package logger

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

// recorder collects the entries a logger hands its transport.
type recorder struct{ entries []Entry }

func (r *recorder) transport(_ io.Writer, e Entry) { r.entries = append(r.entries, e) }

func newRecorded() (*Logger, *recorder) {
	r := &recorder{}

	return New(r.transport), r
}

func TestLevels(t *testing.T) {
	for _, logLevel := range []Level{Debug, Info, Warn, Error, Fatal, Fatal + 10} {
		l, r := newRecorded()
		l.SetLevel(logLevel)

		l.Debug("debug")
		l.Info("info")
		l.Warn("warn")
		l.Error("error")
		l.Fatal("fatal")

		var got []string

		for _, e := range r.entries {
			got = append(got, e.Message)
		}

		var want []string

		for _, lv := range []Level{Debug, Info, Warn, Error, Fatal} {
			if lv >= logLevel {
				want = append(want, strings.ToLower(levelTags[lv]))
			}
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("at %d: logged %v, want %v", logLevel, got, want)
		}
	}
}

func TestMessages(t *testing.T) {
	t.Run("logs each message of a list", func(t *testing.T) {
		l, r := newRecorded()
		l.Info([]string{"a", "b"})

		if len(r.entries) != 2 || r.entries[0].Message != "a" || r.entries[1].Message != "b" {
			t.Errorf("got %v", r.entries)
		}
	})

	t.Run("logs an error's message", func(t *testing.T) {
		l, r := newRecorded()
		l.Error(errors.New("boom"), Fields{File: &File{Path: "test.md"}})

		if len(r.entries) != 1 || r.entries[0].Message != "boom" || r.entries[0].Level != Error || r.entries[0].Fields.File.Path != "test.md" {
			t.Errorf("got %v", r.entries)
		}
	})
}

func TestSetLevel(t *testing.T) {
	l, r := newRecorded()

	l.SetLevel(Error)
	l.Warn("hidden")

	l.SetLevel("WARN")
	l.Warn("shown")

	// Unknown names are ignored
	l.SetLevel("verbose")
	l.Info("hidden")

	if len(r.entries) != 1 || r.entries[0].Message != "shown" {
		t.Errorf("got %v", r.entries)
	}

	if New(ConsoleTransport).Level() != Info {
		t.Error("the default level should be info")
	}
}

func TestChildren(t *testing.T) {
	t.Run("start at the parent's level, and follow it", func(t *testing.T) {
		parent, r := newRecorded()
		parent.SetLevel(Error)

		child := parent.Child("child")
		grandchild := child.Child("grandchild")
		sibling := parent.Child("sibling")

		if child.Level() != Error {
			t.Errorf("child level %d", child.Level())
		}

		parent.SetLevel(Debug)

		for _, l := range []*Logger{child, grandchild, sibling} {
			if l.Level() != Debug {
				t.Errorf("%s level %d", l.module, l.Level())
			}
		}

		grandchild.Debug("message")

		if len(r.entries) != 1 || r.entries[0].Module != "grandchild" {
			t.Errorf("got %v", r.entries)
		}
	})

	t.Run("don't change their parent's level", func(t *testing.T) {
		parent, _ := newRecorded()
		parent.Child("child").SetLevel(Fatal)

		if parent.Level() != Info {
			t.Errorf("parent level %d", parent.Level())
		}
	})
}

var epoch = time.Unix(0, 0)

func TestConsoleTransport(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")

	cases := []struct {
		name  string
		entry Entry
		want  string
	}{
		{"debug", Entry{Level: Debug, Message: "Test message"}, "[00:00:00.000] \x1b[34mDEBUG\x1b[39m: Test message\n"},
		{"info", Entry{Level: Info, Message: "Test message"}, "[00:00:00.000] \x1b[32mINFO\x1b[39m: Test message\n"},
		{"error", Entry{Level: Error, Message: "Test message"}, "[00:00:00.000] \x1b[35mERROR\x1b[39m: Test message\n"},
		{"fatal", Entry{Level: Fatal, Message: "Test message"}, "[00:00:00.000] \x1b[31mFATAL\x1b[39m: Test message\n"},
		{"a file", Entry{Level: Info, Message: "Test message", Fields: Fields{File: &File{Path: "test.md", Position: &Position{Start: 1, End: 1}}}}, "[00:00:00.000] \x1b[32mINFO\x1b[39m: Test message at test.md(1:1)\n"},
		{"a module", Entry{Level: Info, Message: "Test message", Module: "child1"}, "[00:00:00.000] \x1b[32mINFO\x1b[39m (child1): Test message\n"},
		{"metadata", Entry{Level: Info, Message: "Test message", Fields: With("threads", 4, "generator", "metadata")}, "[00:00:00.000] \x1b[32mINFO\x1b[39m: Test message \x1b[35m{\"threads\":4,\"generator\":\"metadata\"}\x1b[39m\n"},
		{"a file and a stack", Entry{Level: Info, Message: "Test message", Fields: Fields{File: &File{Path: "test.md"}, Stack: "Error: test\n  at test.mjs:1:1"}}, "[00:00:00.000] \x1b[32mINFO\x1b[39m: Test message at test.md\nError: test\n  at test.mjs:1:1"},
		{"a file and metadata", Entry{Level: Debug, Message: "Processing chunk", Fields: Fields{File: &File{Path: "api.md", Position: &Position{Start: 10, End: 20}}, Extra: []any{"chunkId", 3, "itemCount", 15}}}, "[00:00:00.000] \x1b[34mDEBUG\x1b[39m: Processing chunk at api.md(10:20) \x1b[35m{\"chunkId\":3,\"itemCount\":15}\x1b[39m\n"},
	}

	for _, c := range cases {
		var b strings.Builder

		c.entry.Timestamp = epoch
		ConsoleTransport(&b, c.entry)

		if b.String() != c.want {
			t.Errorf("%s: got %q, want %q", c.name, b.String(), c.want)
		}
	}

	t.Run("without colors when FORCE_COLOR is 0", func(t *testing.T) {
		t.Setenv("FORCE_COLOR", "0")

		var b strings.Builder

		ConsoleTransport(&b, Entry{Level: Info, Message: "Test message", Timestamp: epoch})

		if b.String() != "[00:00:00.000] INFO: Test message\n" {
			t.Errorf("got %q", b.String())
		}
	})
}

func TestGitHubTransport(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")

	cases := []struct {
		name  string
		entry Entry
		want  string
	}{
		{"debug", Entry{Level: Debug, Message: "Test message"}, "::debug::[00:00:00.000] \x1b[34mDEBUG\x1b[39m: Test message"},
		{"info", Entry{Level: Info, Message: "Test message"}, "::notice ::[00:00:00.000] \x1b[32mINFO\x1b[39m: Test message"},
		{"error", Entry{Level: Error, Message: "Test message"}, "::error ::[00:00:00.000] \x1b[35mERROR\x1b[39m: Test message"},
		{"fatal", Entry{Level: Fatal, Message: "Test message"}, "::error ::[00:00:00.000] \x1b[31mFATAL\x1b[39m: Test message"},
		{"a file", Entry{Level: Info, Message: "Test message", Fields: Fields{File: &File{Path: "test.md", Position: &Position{Start: 1, End: 1}}}}, "::notice file=test.md,line=1,endLine=1::[00:00:00.000] \x1b[32mINFO\x1b[39m: Test message"},
		{"escapes", Entry{Level: Info, Message: "100%: a, b\r\nc", Fields: Fields{File: &File{Path: "a,b:c%.md", Position: &Position{Start: 1, End: 2}}}}, "::notice file=a%2Cb%3Ac%25.md,line=1,endLine=2::[00:00:00.000] \x1b[32mINFO\x1b[39m: 100%25: a, b%0D%0Ac"},
		{"a module", Entry{Level: Info, Message: "Test message", Module: "child1"}, "::notice ::[00:00:00.000] \x1b[32mINFO\x1b[39m (child1): Test message"},
	}

	for _, c := range cases {
		var b strings.Builder

		c.entry.Timestamp = epoch
		GitHubTransport(&b, c.entry)

		if got := strings.TrimSpace(b.String()); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}

	t.Run("without colors when FORCE_COLOR is 0", func(t *testing.T) {
		t.Setenv("FORCE_COLOR", "0")

		var b strings.Builder

		GitHubTransport(&b, Entry{Level: Info, Message: "Test message", Timestamp: epoch})

		if got := strings.TrimSpace(b.String()); got != "::notice ::[00:00:00.000] INFO: Test message" {
			t.Errorf("got %q", got)
		}
	})
}
