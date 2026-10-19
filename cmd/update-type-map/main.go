// Command update-type-map refreshes the map of MDN type links (`mdn.json`)
// from MDN's browser compatibility data.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/nodejs/doc-kit/internal/omap"
)

// compatURL is the latest release of MDN's browser compatibility data.
const compatURL = "https://github.com/mdn/browser-compat-data/releases/latest/download/data.json"

// outputs are the copies of the map: the Go generators' and the JavaScript
// package's.
var outputs = []string{
	"internal/metadata/maps/mdn.json",
	"packages/core/src/generators/metadata/maps/mdn.json",
}

type feature struct {
	Compat struct {
		MDNURL string `json:"mdn_url"`
	} `json:"__compat"`
}

// addFeatures adds each feature of an object, in order, by its lowercased
// name, to the map, when it has an MDN page.
func addFeatures(m *omap.Map, raw json.RawMessage) error {
	dec := json.NewDecoder(bytes.NewReader(raw))

	if _, err := dec.Token(); err != nil {
		return err
	}

	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return err
		}

		var f feature

		if err := dec.Decode(&f); err != nil {
			return err
		}

		if f.Compat.MDNURL != "" {
			m.Set(strings.ToLower(key.(string)), f.Compat.MDNURL)
		}
	}

	return nil
}

func run() error {
	resp, err := http.Get(compatURL)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var compat struct {
		API        json.RawMessage `json:"api"`
		JavaScript struct {
			Builtins json.RawMessage `json:"builtins"`
		} `json:"javascript"`
	}

	if err := json.Unmarshal(data, &compat); err != nil {
		return err
	}

	m := omap.New()

	for _, raw := range []json.RawMessage{compat.API, compat.JavaScript.Builtins} {
		if err := addFeatures(m, raw); err != nil {
			return err
		}
	}

	out, err := omap.Stringify(m, 2)
	if err != nil {
		return err
	}

	for _, path := range outputs {
		if err := os.WriteFile(path, out, 0o644); err != nil {
			return err
		}
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
