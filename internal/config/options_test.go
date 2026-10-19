package config

import "testing"

func TestPopulate(t *testing.T) {
	t.Run("substitutes template variables from the options", func(t *testing.T) {
		got := Populate("https://github.com/{repository}/blob/{ref}/", Options{"repository": "nodejs/node", "ref": "main"})

		if got != "https://github.com/nodejs/node/blob/main/" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("uses the fallback when the value is missing", func(t *testing.T) {
		if got := Populate("{name|unknown}", Options{}); got != "unknown" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("leaves the placeholder when there is no value or fallback", func(t *testing.T) {
		if got := Populate("{missing}", Options{}); got != "{missing}" {
			t.Errorf("got %q", got)
		}
	})
}
