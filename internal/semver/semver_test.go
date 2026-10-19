package semver

import "testing"

func TestCoerce(t *testing.T) {
	cases := map[string]string{
		"1.2.3":            "1.2.3",
		"v26.0.0":          "26.0.0",
		"v22":              "22.0.0",
		"0.0.0-REPLACEME":  "0.0.0",
		"invalid":          "0.0.0",
		"":                 "0.0.0",
		"Node.js 18.5":     "18.5.0",
		"prefix 1.2.3.4.5": "1.2.3",
	}

	for in, want := range cases {
		if got := MustCoerce(in).String(); got != want {
			t.Errorf("MustCoerce(%q) = %q, want %q", in, got, want)
		}
	}
}
