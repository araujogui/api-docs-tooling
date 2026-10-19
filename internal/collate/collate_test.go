package collate

import (
	"sort"
	"testing"
)

func TestCompare(t *testing.T) {
	names := []string{"Zlib", "assert", "Buffer", "child_process", "cli", "Child processes", "http2", "http", "C++ addons", "_http_agent"}

	sort.SliceStable(names, func(i, j int) bool { return Compare(names[i], names[j]) < 0 })

	// As String.prototype.localeCompare sorts them
	want := []string{"_http_agent", "assert", "Buffer", "C++ addons", "Child processes", "child_process", "cli", "http", "http2", "Zlib"}

	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("got %v, want %v", names, want)
		}
	}

	if Compare("a", "A") != -1 || Compare("A", "a") != 1 || Compare("a", "a") != 0 {
		t.Error("lowercase sorts first on ties")
	}
}
