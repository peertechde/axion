package starlark

import (
	"fmt"

	"go.starlark.net/starlark"
)

// NewFile returns a starlark.Builtin for creating File resources
func NewFile() *starlark.Builtin {
	return starlark.NewBuiltin("file", newFile)
}

func newFile(
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

	file := &File{
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
		file.Dependencies = deps
	}

	return file, nil
}

type File struct {
	State        string
	Path         string
	Mode         string
	Owner        string
	Group        string
	Dependencies []starlark.Value
}

func (f *File) Attr(name string) (starlark.Value, error) {
	switch name {
	case "state":
		return starlark.String(f.State), nil
	case "path":
		return starlark.String(f.Path), nil
	case "mode":
		return starlark.String(f.Mode), nil
	case "owner":
		return starlark.String(f.Owner), nil
	case "group":
		return starlark.String(f.Group), nil
	case "dependencies":
		deps := make([]starlark.Value, len(f.Dependencies))
		copy(deps, f.Dependencies)
		return starlark.NewList(deps), nil
	default:
		return nil, nil
	}
}

func (f *File) Id() string {
	return "file:" + f.Path
}

func (f *File) AttrNames() []string {
	return []string{"state", "path", "mode", "owner", "group", "dependencies"}
}

func (f *File) Type() string {
	return "file"
}

func (f *File) Freeze() {
	// Freeze dependencies as well
	for _, dep := range f.Dependencies {
		dep.Freeze()
	}
}

func (f *File) Truth() starlark.Bool {
	return starlark.True
}

func (f *File) Hash() (uint32, error) {
	return 0, fmt.Errorf("file is unhashable")
}

func (f *File) String() string {
	return f.Id()
}

func (f *File) GetDependencies() []starlark.Value {
	deps := make([]starlark.Value, len(f.Dependencies))
	copy(deps, f.Dependencies)
	return deps
}
