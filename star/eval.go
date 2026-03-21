// SPDX-License-Identifier: GPL-3.0-only

package star

import (
	"context"
	"path/filepath"
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"scampi.dev/scampi/source"
	"scampi.dev/scampi/spec"
)

const maxExecutionSteps = 100_000_000

// fileOptions enables language extensions beyond core Starlark: set literals,
// while loops, and recursive functions. All are useful for config generation
// and don't compromise hermeticity.
var fileOptions = &syntax.FileOptions{
	Set:       true,
	While:     true,
	Recursion: true,
}

// Eval loads and evaluates a Starlark configuration file at cfgPath.
func Eval(
	ctx context.Context,
	cfgPath string,
	store *spec.SourceStore,
	src source.Source,
) (spec.Config, error) {
	data, err := src.ReadFile(ctx, cfgPath)
	if err != nil {
		return spec.Config{}, &FileReadError{Path: cfgPath, Cause: err}
	}

	if store != nil {
		store.AddFile(cfgPath, data)
	}

	collector := newCollector(ctx, cfgPath, src)

	thread := &starlark.Thread{
		Name: cfgPath,
		Load: makeLoad(ctx, cfgPath, src, store),
	}
	thread.SetLocal(collectorKey, collector)
	thread.SetMaxExecutionSteps(maxExecutionSteps)

	// Wire context cancellation into thread cancellation.
	go func() {
		<-ctx.Done()
		thread.Cancel(ctx.Err().Error())
	}()

	f, prog, err := starlark.SourceProgramOptions(fileOptions, cfgPath, data, predeclared().Has)
	if err != nil {
		return spec.Config{}, wrapStarlarkError(err, collector)
	}
	collector.AddAST(cfgPath, f)

	_, err = prog.Init(thread, predeclared())
	if err != nil {
		return spec.Config{}, wrapStarlarkError(err, collector)
	}

	return collector.Config(), nil
}

// makeLoad returns a load() handler that resolves paths relative to the
// currently executing file, reads via source.Source, and caches results.
func makeLoad(
	ctx context.Context,
	basePath string,
	src source.Source,
	store *spec.SourceStore,
) func(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	var (
		mu      sync.Mutex
		cache   = make(map[string]*loadEntry)
		loading = make(map[string]bool)
	)

	return func(thread *starlark.Thread, module string) (starlark.StringDict, error) {
		// Resolve relative to the file that called load()
		callerFile := basePath
		if stack := thread.CallStack(); len(stack) > 0 {
			if f := stack[len(stack)-1].Pos.Filename(); f != "" {
				callerFile = f
			}
		}

		absPath := module
		if !filepath.IsAbs(module) {
			absPath = filepath.Join(filepath.Dir(callerFile), module)
		}

		mu.Lock()
		if loading[absPath] {
			mu.Unlock()
			return nil, &CircularLoadError{
				Path:   absPath,
				Source: callSpan(thread),
			}
		}
		entry, ok := cache[absPath]
		if ok {
			mu.Unlock()
			return entry.globals, entry.err
		}
		loading[absPath] = true
		mu.Unlock()

		globals, err := execModule(ctx, thread, absPath, src, store)

		mu.Lock()
		delete(loading, absPath)
		cache[absPath] = &loadEntry{globals: globals, err: err}
		mu.Unlock()

		return globals, err
	}
}

type loadEntry struct {
	globals starlark.StringDict
	err     error
}

func execModule(
	ctx context.Context,
	parentThread *starlark.Thread,
	absPath string,
	src source.Source,
	store *spec.SourceStore,
) (starlark.StringDict, error) {
	data, err := src.ReadFile(ctx, absPath)
	if err != nil {
		return nil, &FileReadError{Path: absPath, Cause: err}
	}

	if store != nil {
		store.AddFile(absPath, data)
	}

	// Loaded modules share the same collector (thread-locals) from the parent.
	childThread := &starlark.Thread{
		Name: absPath,
		Load: parentThread.Load,
	}
	childThread.SetLocal(collectorKey, parentThread.Local(collectorKey))
	childThread.SetMaxExecutionSteps(maxExecutionSteps)

	fOpts := &syntax.FileOptions{
		Set:       true,
		While:     true,
		Recursion: true,
	}

	collector := threadCollector(parentThread)

	f, prog, err := starlark.SourceProgramOptions(fOpts, absPath, data, predeclared().Has)
	if err != nil {
		return nil, wrapStarlarkError(err, collector)
	}
	collector.AddAST(absPath, f)

	globals, err := prog.Init(childThread, predeclared())
	if err != nil {
		return nil, wrapStarlarkError(err, collector)
	}

	return globals, nil
}
