package generators

import (
	"fmt"
	"slices"
	"sync"

	"github.com/nodejs/doc-kit/internal/logger"
)

// splice rewires a dependent's pipeline so the first generator (walking up
// from the dependent) that consumes the splicing generator's own input
// consumes the splicing generator instead.
func splice(name, dependent string, inputOf map[string]string, displayName string) error {
	upstream := inputOf[name]
	visited := map[string]bool{}
	current := dependent

	for current != "" && current != name {
		if visited[current] {
			break
		}

		visited[current] = true

		if inputOf[current] == upstream {
			inputOf[current] = name

			return nil
		}

		current = inputOf[current]
	}

	if upstream == "" {
		upstream = "no input"
	}

	return fmt.Errorf("generator %q declares %q as its dependent, but nothing in that pipeline consumes %q, so there is nowhere to splice it in", displayName, dependent, upstream)
}

func dependencyClosure(targets []string, generators map[string]*Generator) map[string]bool {
	closure := map[string]bool{}
	stack := append([]string{}, targets...)

	for len(stack) > 0 {
		name := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if closure[name] {
			continue
		}

		closure[name] = true

		if g, ok := generators[name]; ok && g.DependsOn != "" {
			stack = append(stack, Resolve(g.DependsOn))
		}
	}

	return closure
}

func resolveTerminals(name string, generators map[string]*Generator, requested, seen map[string]bool) []string {
	if seen[name] {
		return nil
	}

	seen[name] = true

	var dependents []string

	if g, ok := generators[name]; ok {
		for _, d := range g.Dependent {
			dependents = append(dependents, Resolve(d))
		}
	}

	if len(dependents) == 0 {
		return []string{name}
	}

	var active []string

	for _, d := range dependents {
		if requested[d] {
			active = append(active, d)
		}
	}

	if len(active) == 0 {
		active = dependents
	}

	var terminals []string

	for _, d := range active {
		terminals = append(terminals, resolveTerminals(d, generators, requested, seen)...)
	}

	return terminals
}

// ResolvePipeline works out which generator each generator reads its input
// from (its dependency, unless a generator declaring it as a dependent was
// spliced in front of it), and which generators the run collects (a requested
// generator with dependents is delivered through them).
func ResolvePipeline(targets []string, generators map[string]*Generator, order []string) ([]string, map[string]string, error) {
	inputOf := map[string]string{}

	for _, name := range order {
		if g := generators[name]; g.DependsOn != "" {
			inputOf[name] = Resolve(g.DependsOn)
		} else {
			inputOf[name] = ""
		}
	}

	for _, name := range order {
		for _, d := range generators[name].Dependent {
			if err := splice(name, Resolve(d), inputOf, generators[name].Name); err != nil {
				return nil, nil, err
			}
		}
	}

	requested := dependencyClosure(targets, generators)

	var collected []string

	for _, target := range targets {
		for _, terminal := range resolveTerminals(target, generators, requested, map[string]bool{}) {
			if !slices.Contains(collected, terminal) {
				collected = append(collected, terminal)
			}
		}
	}

	return collected, inputOf, nil
}

type result struct {
	once  sync.Once
	value any
	err   error
}

// Run runs the requested generators, and their dependencies, returning the
// requested generators' outputs. Independent generators run concurrently,
// and each generator runs once however many others consume it.
func Run(cfg *Configuration) ([]any, error) {
	log := logger.Default.Child("generators")

	targets := make([]string, len(cfg.Target))

	for i, t := range cfg.Target {
		targets[i] = Resolve(t)
	}

	generators, err := Load(targets)
	if err != nil {
		return nil, err
	}

	order := make([]string, 0, len(generators))

	for name := range generators {
		order = append(order, name)
	}

	slices.Sort(order)

	collected, inputOf, err := ResolvePipeline(targets, generators, order)
	if err != nil {
		return nil, err
	}

	log.Debug("Starting pipeline", logger.With("generators", collected, "threads", cfg.Threads))

	results := map[string]*result{}

	for name := range generators {
		results[name] = &result{}
	}

	var run func(name string) (any, error)

	run = func(name string) (any, error) {
		r := results[name]

		r.once.Do(func() {
			var input any

			if dep := inputOf[name]; dep != "" {
				input, r.err = run(dep)

				if r.err != nil {
					return
				}
			}

			g := generators[name]
			log.Debug(fmt.Sprintf("Starting %q", g.Name))

			r.value, r.err = g.Generate(&Context{Options: cfg.Options(g.Name), Run: cfg, Log: logger.Default.Child(g.Name)}, input)

			if r.err == nil {
				log.Debug(fmt.Sprintf("Completed %q", g.Name))
			} else {
				r.err = fmt.Errorf("%s: %w", g.Name, r.err)
			}
		})

		return r.value, r.err
	}

	values := make([]any, len(collected))
	errs := make([]error, len(collected))

	var wg sync.WaitGroup

	for i, name := range collected {
		wg.Add(1)

		go func() {
			defer wg.Done()

			values[i], errs[i] = run(name)
		}()
	}

	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	return values, nil
}
