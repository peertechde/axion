package starlark

import (
	"context"
	"fmt"
	"os"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"

	"peertech.de/axion/pkg/config"
	"peertech.de/axion/pkg/orchestrator"
	"peertech.de/axion/pkg/resource"
)

// Declaration of resources
var resources = starlarkstruct.FromStringDict(
	starlark.String("resources"),
	starlark.StringDict{
		"command":   NewCommand(),
		"directory": NewDirectory(),
		"file":      NewFile(),
	},
)

// Loader implements the manifest.Loader interface for Starlark-based manifests
type Loader struct{}

// Load executes a Starlark script and extracts resource specifications
func (l *Loader) Load(ctx context.Context, cfg *config.Config, path string) ([]orchestrator.ResourceSpec, error) {
	r := NewRuntime(nil)

	globals, err := r.Load(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("starlark execution error: %w", err)
	}

	return l.extractResources(cfg, globals)
}

// extractResources converts Starlark values to orchestrator resource specs
func (l *Loader) extractResources(cfg *config.Config, globals starlark.StringDict) ([]orchestrator.ResourceSpec, error) {
	// Discover all resources and map them to their variable names.
	resources := make(map[string]Resource)
	reverse := make(map[Resource]string)

	for name, value := range globals {
		if res, ok := value.(Resource); ok {
			resources[name] = res
			reverse[res] = name
		}
	}

	// Resolve dependencies and build the final ResourceSpec list
	var specs []orchestrator.ResourceSpec
	for name, obj := range resources {
		// Convert the Starlark resource to a concrete orchestrator resource
		res, ok := l.convertToResource(cfg, obj)
		if !ok {
			return nil, fmt.Errorf("failed to convert starlark resource %q", name)
		}

		// Resolve dependencies.
		var ids []string
		deps := obj.GetDependencies()
		for _, dep := range deps {
			if depRes, ok := dep.(Resource); ok {
				// Look up the dependency's variable name
				name, found := reverse[depRes]
				if !found {
					return nil, fmt.Errorf("resource %q depends on an unregistered resource object (%s)", name, dep.String())
				}
				ids = append(ids, name)
			}
		}

		spec := orchestrator.ResourceSpec{
			Id:           name, // The Id is the Starlark variable name
			Resource:     res,
			Dependencies: ids,
		}
		specs = append(specs, spec)
	}

	return specs, nil
}

// convertToResource attempts to convert a Starlark value to a concrete resource
func (l *Loader) convertToResource(cfg *config.Config, value starlark.Value) (resource.Resource, bool) {
	switch v := value.(type) {
	case *Command:
		// TODO: isConcurrent, timeout, expectedExitCodes
		return resource.NewCommand(
			cfg,
			v.Command,
		), true
	case *File:
		return resource.NewFile(
			cfg,
			resource.State(v.State),
			v.Path,
			optionalString(v.Mode),
			optionalString(v.Owner),
			optionalString(v.Group),
		), true
	case *Directory:
		return resource.NewDirectory(
			cfg,
			resource.State(v.State),
			v.Path,
			optionalString(v.Mode),
			optionalString(v.Owner),
			optionalString(v.Group),
		), true
	default:
		return nil, false
	}
}

func NewRuntime(extra starlark.StringDict) *Runtime {
	globals := starlark.StringDict{
		"struct":    MakeStruct,
		"resources": resources,
	}

	// Add extra predeclared values
	for k, v := range extra {
		globals[k] = v
	}

	return &Runtime{
		opts:    &syntax.FileOptions{},
		globals: globals,
	}
}

type Runtime struct {
	opts    *syntax.FileOptions
	globals starlark.StringDict
}

func (r *Runtime) Load(ctx context.Context, path string) (starlark.StringDict, error) {
	src, err := load(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load module: %w", err)
	}

	return r.Run(ctx, src)
}

func (r *Runtime) Run(ctx context.Context, src string) (starlark.StringDict, error) {
	thread := r.thread(ctx)
	return starlark.ExecFileOptions(r.opts, thread, "main", src, r.globals)
}

func (r *Runtime) thread(ctx context.Context) *starlark.Thread {
	thread := &starlark.Thread{
		Print: func(thread *starlark.Thread, msg string) {
			pos := thread.CallFrame(1).Pos
			fmt.Fprintf(os.Stderr, "[%s:%d] %s\n", pos.Filename(), pos.Line, msg)
		},
	}

	return thread
}

// GetResources extracts all resources from the execution result
func (r *Runtime) GetResources(globals starlark.StringDict) map[string]Resource {
	resources := make(map[string]Resource)

	for name, value := range globals {
		if res, ok := value.(Resource); ok {
			resources[name] = res
		}
	}

	return resources
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
