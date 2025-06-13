package starlark

import (
	"fmt"

	"go.starlark.net/starlark"
)

// NewDirectory returns a starlark.Builtin for creating Directory resources
func NewDirectory() *starlark.Builtin {
	return starlark.NewBuiltin("directory", newDirectory)
}

func newDirectory(
	thread *starlark.Thread,
	b *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var state, path starlark.String
	var mode, owner, group starlark.String
	var dependencies *starlark.List

	err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"state", &state,
		"path", &path,
		"mode?", &mode,
		"owner?", &owner,
		"group?", &group,
		"dependencies?", &dependencies,
	)
	if err != nil {
		return nil, err
	}

	// Validate required fields
	if string(state) == "" {
		return nil, fmt.Errorf("state cannot be empty")
	}
	if string(path) == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}

	dir := &Directory{
		State: string(state),
		Path:  string(path),
		Mode:  string(mode),
		Owner: string(owner),
		Group: string(group),
	}

	// Parse dependencies as resource values
	if dependencies != nil {
		deps, err := parseDependencies(dependencies)
		if err != nil {
			return nil, fmt.Errorf("invalid dependencies: %w", err)
		}
		dir.Dependencies = deps
	}

	return dir, nil
}

type Directory struct {
	State        string
	Path         string
	Mode         string
	Owner        string
	Group        string
	Dependencies []starlark.Value
}

func (d *Directory) Attr(name string) (starlark.Value, error) {
	switch name {
	case "state":
		return starlark.String(d.State), nil
	case "path":
		return starlark.String(d.Path), nil
	case "mode":
		return starlark.String(d.Mode), nil
	case "owner":
		return starlark.String(d.Owner), nil
	case "group":
		return starlark.String(d.Group), nil
	case "dependencies":
		deps := make([]starlark.Value, len(d.Dependencies))
		copy(deps, d.Dependencies)
		return starlark.NewList(deps), nil
	default:
		return nil, nil
	}
}

func (d *Directory) Id() string {
	return "directory:" + d.Path
}

func (d *Directory) AttrNames() []string {
	return []string{"state", "path", "mode", "owner", "group", "dependencies"}
}

func (d *Directory) Type() string {
	return "directory"
}

func (d *Directory) Freeze() {
	// Freeze dependencies as well
	for _, dep := range d.Dependencies {
		dep.Freeze()
	}
}

func (d *Directory) Truth() starlark.Bool {
	return starlark.True
}

func (d *Directory) Hash() (uint32, error) {
	return 0, fmt.Errorf("directory is unhashable")
}

func (d *Directory) String() string {
	return d.Id()
}

func (d *Directory) GetDependencies() []starlark.Value {
	deps := make([]starlark.Value, len(d.Dependencies))
	copy(deps, d.Dependencies)
	return deps
}
