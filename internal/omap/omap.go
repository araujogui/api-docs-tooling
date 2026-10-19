// Package omap provides an insertion-ordered map, standing in for the plain
// JavaScript objects the original tooling built dynamically. Key order is
// part of the generated JSON, so it has to be preserved.
package omap

import (
	"bytes"
	"encoding/json"
)

// undefined marks a key that exists (and keeps its position) but is left out
// of JSON output, as JavaScript does with `undefined` values.
type undefined struct{}

// Undefined is the value of a key that is present but serialises to nothing.
var Undefined any = undefined{}

// Map is an insertion-ordered string-keyed map.
type Map struct {
	keys []string
	vals map[string]any
}

// New creates a map from alternating key/value pairs.
func New(pairs ...any) *Map {
	m := &Map{vals: map[string]any{}}

	for i := 0; i+1 < len(pairs); i += 2 {
		m.Set(pairs[i].(string), pairs[i+1])
	}

	return m
}

// Set assigns a value, appending the key if it is new.
func (m *Map) Set(key string, value any) *Map {
	if m.vals == nil {
		m.vals = map[string]any{}
	}

	if _, ok := m.vals[key]; !ok {
		m.keys = append(m.keys, key)
	}

	m.vals[key] = value

	return m
}

// Get returns a key's value, and whether the key exists.
func (m *Map) Get(key string) (any, bool) {
	if m == nil {
		return nil, false
	}

	v, ok := m.vals[key]

	return v, ok
}

// Value returns a key's value, or nil when it is absent or undefined.
func (m *Map) Value(key string) any {
	v, ok := m.Get(key)

	if !ok || v == Undefined {
		return nil
	}

	return v
}

// Has reports whether the key exists (even when undefined), like `in`.
func (m *Map) Has(key string) bool {
	_, ok := m.Get(key)

	return ok
}

// Delete removes a key.
func (m *Map) Delete(key string) {
	if _, ok := m.vals[key]; !ok {
		return
	}

	delete(m.vals, key)

	for i, k := range m.keys {
		if k == key {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			break
		}
	}
}

// Keys returns the keys in insertion order.
func (m *Map) Keys() []string {
	if m == nil {
		return nil
	}

	return m.keys
}

// Len returns the number of keys.
func (m *Map) Len() int {
	if m == nil {
		return 0
	}

	return len(m.keys)
}

// Clone returns a shallow copy.
func (m *Map) Clone() *Map {
	c := New()

	for _, k := range m.Keys() {
		c.Set(k, m.vals[k])
	}

	return c
}

// MarshalJSON writes the map as a JSON object, skipping undefined values.
func (m *Map) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}

	var buf bytes.Buffer

	buf.WriteByte('{')

	first := true

	for _, k := range m.keys {
		v := m.vals[k]

		if v == Undefined {
			continue
		}

		if !first {
			buf.WriteByte(',')
		}

		first = false

		key, err := Marshal(k)
		if err != nil {
			return nil, err
		}

		buf.Write(key)
		buf.WriteByte(':')

		val, err := Marshal(v)
		if err != nil {
			return nil, err
		}

		buf.Write(val)
	}

	buf.WriteByte('}')

	return buf.Bytes(), nil
}

// Marshal encodes a value as compact JSON the way JSON.stringify does: no
// HTML escaping, and undefined array items become null.
func Marshal(v any) ([]byte, error) {
	if v == Undefined {
		return []byte("null"), nil
	}

	if arr, ok := v.([]any); ok {
		var buf bytes.Buffer

		buf.WriteByte('[')

		for i, item := range arr {
			if i > 0 {
				buf.WriteByte(',')
			}

			b, err := Marshal(item)
			if err != nil {
				return nil, err
			}

			buf.Write(b)
		}

		buf.WriteByte(']')

		return buf.Bytes(), nil
	}

	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(v); err != nil {
		return nil, err
	}

	return unescapeLineSeparators(bytes.TrimRight(buf.Bytes(), "\n")), nil
}

// Stringify encodes a value like JSON.stringify(value, null, indent).
func Stringify(v any, indent int) ([]byte, error) {
	compact, err := Marshal(v)
	if err != nil {
		return nil, err
	}

	if indent == 0 {
		return compact, nil
	}

	var buf bytes.Buffer

	if err := json.Indent(&buf, compact, "", string(bytes.Repeat([]byte{' '}, indent))); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// unescapeLineSeparators undoes Go's escaping of U+2028 and U+2029, which
// JSON.stringify writes as they are.
func unescapeLineSeparators(b []byte) []byte {
	if !bytes.Contains(b, []byte(`\u202`)) {
		return b
	}

	out := make([]byte, 0, len(b))

	for i := 0; i < len(b); i++ {
		if b[i] == '\\' && i+5 < len(b) && string(b[i+1:i+5]) == "u202" && (b[i+5] == '8' || b[i+5] == '9') {
			if b[i+5] == '8' {
				out = append(out, "\u2028"...)
			} else {
				out = append(out, "\u2029"...)
			}

			i += 5

			continue
		}

		if b[i] == '\\' && i+1 < len(b) {
			// Copy escape sequences whole, so an escaped backslash followed by
			// `u2028` text is not mistaken for an escape of its own
			out = append(out, b[i], b[i+1])
			i++

			continue
		}

		out = append(out, b[i])
	}

	return out
}
