package generators

import (
	"reflect"
	"regexp"
	"sync"
	"testing"
)

func define(defs map[string]*Generator) (map[string]*Generator, []string) {
	var order []string

	for name, g := range defs {
		g.Name = name
		order = append(order, name)
	}

	// Iterate in a stable order
	sortStrings(order)

	return defs, order
}

func sortStrings(s []string) {
	for i := range s {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}

func web(extra map[string]*Generator) map[string]*Generator {
	defs := map[string]*Generator{
		"source": {},
		"parse":  {DependsOn: "source"},
		"render": {DependsOn: "parse"},
		"site":   {DependsOn: "render"},
	}

	for k, v := range extra {
		defs[k] = v
	}

	return defs
}

func resolve(t *testing.T, targets []string, defs map[string]*Generator) ([]string, map[string]string) {
	t.Helper()

	gens, order := define(defs)
	collected, inputOf, err := ResolvePipeline(targets, gens, order)

	if err != nil {
		t.Fatal(err)
	}

	return collected, inputOf
}

func TestResolvePipeline(t *testing.T) {
	t.Run("reads each generator from its dependency by default", func(t *testing.T) {
		targets, inputOf := resolve(t, []string{"site"}, web(nil))

		want := map[string]string{"source": "", "parse": "source", "render": "parse", "site": "render"}

		if !reflect.DeepEqual(targets, []string{"site"}) || !reflect.DeepEqual(inputOf, want) {
			t.Errorf("got %v, %v", targets, inputOf)
		}
	})

	t.Run("splices a generator in front of the consumer of its input", func(t *testing.T) {
		targets, inputOf := resolve(t, []string{"splice"}, web(map[string]*Generator{"splice": {DependsOn: "parse", Dependent: []string{"site"}}}))

		if !reflect.DeepEqual(targets, []string{"site"}) || inputOf["splice"] != "parse" || inputOf["render"] != "splice" || inputOf["site"] != "render" {
			t.Errorf("got %v, %v", targets, inputOf)
		}
	})

	t.Run("collects a dependent once when it is also requested", func(t *testing.T) {
		targets, _ := resolve(t, []string{"site", "splice"}, web(map[string]*Generator{"splice": {DependsOn: "parse", Dependent: []string{"site"}}}))

		if !reflect.DeepEqual(targets, []string{"site"}) {
			t.Errorf("got %v", targets)
		}
	})

	t.Run("chains several generators splicing at the same point", func(t *testing.T) {
		_, inputOf := resolve(t, []string{"first", "second"}, web(map[string]*Generator{
			"first":  {DependsOn: "parse", Dependent: []string{"site"}},
			"second": {DependsOn: "parse", Dependent: []string{"site"}},
		}))

		if inputOf["render"] != "first" || inputOf["first"] != "second" || inputOf["second"] != "parse" {
			t.Errorf("got %v", inputOf)
		}
	})

	t.Run("can splice a source in front of the pipeline root", func(t *testing.T) {
		_, inputOf := resolve(t, []string{"feed"}, web(map[string]*Generator{"feed": {Dependent: []string{"site"}}}))

		if inputOf["source"] != "feed" || inputOf["feed"] != "" {
			t.Errorf("got %v", inputOf)
		}
	})

	t.Run("leaves unrelated pipelines untouched", func(t *testing.T) {
		_, inputOf := resolve(t, []string{"splice", "other"}, web(map[string]*Generator{
			"other":  {DependsOn: "parse"},
			"splice": {DependsOn: "parse", Dependent: []string{"site"}},
		}))

		if inputOf["other"] != "parse" {
			t.Errorf("got %v", inputOf)
		}
	})

	t.Run("delivers through every dependent when none is in the run", func(t *testing.T) {
		targets, inputOf := resolve(t, []string{"splice"}, web(map[string]*Generator{
			"map":    {DependsOn: "parse"},
			"splice": {DependsOn: "parse", Dependent: []string{"site", "map"}},
		}))

		if !reflect.DeepEqual(targets, []string{"site", "map"}) || inputOf["render"] != "splice" || inputOf["map"] != "splice" {
			t.Errorf("got %v, %v", targets, inputOf)
		}
	})

	t.Run("delivers only through the dependents in the run", func(t *testing.T) {
		targets, inputOf := resolve(t, []string{"site", "splice"}, web(map[string]*Generator{
			"map":    {DependsOn: "parse"},
			"splice": {DependsOn: "parse", Dependent: []string{"site", "map"}},
		}))

		if !reflect.DeepEqual(targets, []string{"site"}) || inputOf["map"] != "splice" {
			t.Errorf("got %v, %v", targets, inputOf)
		}
	})

	t.Run("fails when the dependent pipeline never consumes the input", func(t *testing.T) {
		gens, order := define(web(map[string]*Generator{
			"aside":  {},
			"splice": {DependsOn: "aside", Dependent: []string{"site"}},
		}))

		_, _, err := ResolvePipeline([]string{"splice"}, gens, order)

		if err == nil || !regexp.MustCompile(`nothing in that pipeline consumes "aside"`).MatchString(err.Error()) {
			t.Errorf("got %v", err)
		}
	})
}

func TestRun(t *testing.T) {
	var mu sync.Mutex

	calls := map[string]int{}

	for name, dep := range map[string]string{"test-source": "", "test-a": "test-source", "test-b": "test-source"} {
		name, dep := name, dep

		Register(&Generator{Name: name, DependsOn: dep, Generate: func(_ *Context, input any) (any, error) {
			mu.Lock()
			calls[name]++
			mu.Unlock()

			s, _ := input.(string)

			return s + "/" + name, nil
		}})
	}

	values, err := Run(&Configuration{Target: []string{"test-a", "test-b"}, Threads: 2, generators: nil})
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(values, []any{"/test-source/test-a", "/test-source/test-b"}) {
		t.Errorf("got %v", values)
	}

	// A dependency runs once however many generators consume it
	if calls["test-source"] != 1 {
		t.Errorf("the source ran %d times", calls["test-source"])
	}
}
