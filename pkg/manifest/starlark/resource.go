package starlark

import (
	"fmt"

	"go.starlark.net/starlark"
)

type Resource interface {
	starlark.Value
	starlark.HasAttrs

	// Id returns a unique identifier for this resource
	Id() string

	// TODO: GetDependencies
	GetDependencies() []starlark.Value
}

// isResource can now use the interface
func isResource(v starlark.Value) bool {
	_, ok := v.(Resource)
	return ok
}

// parseDependencies extracts resource values from a Starlark list
func parseDependencies(list *starlark.List) ([]starlark.Value, error) {
	deps := make([]starlark.Value, list.Len())
	for i := 0; i < list.Len(); i++ {
		item := list.Index(i)
		if !isResource(item) {
			return nil, fmt.Errorf("dependency at index %d is not a resource type, got %s", i, item.Type())
		}
		deps[i] = item
	}
	return deps, nil
}
