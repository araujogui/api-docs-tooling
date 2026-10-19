// Package generators defines generators, the pipeline that runs them in
// dependency order, and the configuration of a run.
package generators

import (
	"fmt"
	"sort"
	"sync"

	"github.com/nodejs/doc-kit/internal/config"
	"github.com/nodejs/doc-kit/internal/logger"
)

// Context is what a generator runs with.
type Context struct {
	// Options are the generator's settings, global ones included.
	Options config.Options
	// Run is the whole run's configuration.
	Run *Configuration
	// Log is the generator's logger.
	Log *logger.Logger
}

// Generator turns its dependency's output into its own.
type Generator struct {
	// Name is the generator's name, which keys its configuration.
	Name string
	// Description says what the generator produces.
	Description string
	// DependsOn names the generator whose output this one takes.
	DependsOn string
	// Dependent names the generators this one's output is delivered through
	// (it is spliced in front of them).
	Dependent []string
	// Public generators can be requested from the CLI.
	Public bool
	// Defaults returns the generator's default settings, given the user's
	// configuration.
	Defaults func(user map[string]any) map[string]any
	// Generate produces the generator's output.
	Generate func(ctx *Context, input any) (any, error)
}

var (
	registryMu sync.RWMutex
	registry   = map[string]*Generator{}
	aliases    = map[string]string{}
)

// Register adds a generator to the registry.
func Register(g *Generator) {
	registryMu.Lock()
	defer registryMu.Unlock()

	registry[g.Name] = g
}

// Alias makes a former name resolve to a generator.
func Alias(from, to string) {
	registryMu.Lock()
	defer registryMu.Unlock()

	aliases[from] = to
}

// Resolve turns a target into a generator name, following aliases.
func Resolve(name string) string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	if to, ok := aliases[name]; ok {
		return to
	}

	return name
}

// Get returns a registered generator.
func Get(name string) (*Generator, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	g, ok := registry[Resolve(name)]

	return g, ok
}

// PublicNames lists the generators the CLI can request, sorted.
func PublicNames() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	var names []string

	for name, g := range registry {
		if g.Public {
			names = append(names, name)
		}
	}

	sort.Strings(names)

	return names
}

// Load returns the given generators plus the closure of their dependencies
// and dependents.
func Load(targets []string) (map[string]*Generator, error) {
	loaded := map[string]*Generator{}
	queue := append([]string{}, targets...)

	for len(queue) > 0 {
		name := Resolve(queue[0])
		queue = queue[1:]

		if _, ok := loaded[name]; ok {
			continue
		}

		g, ok := Get(name)

		if !ok {
			return nil, fmt.Errorf("could not load generator %q", name)
		}

		loaded[name] = g

		if g.DependsOn != "" {
			queue = append(queue, g.DependsOn)
		}

		queue = append(queue, g.Dependent...)
	}

	return loaded, nil
}
