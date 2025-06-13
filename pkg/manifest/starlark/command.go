package starlark

import (
	"fmt"

	"go.starlark.net/starlark"
)

// NewCommand returns a starlark.Builtin for creating Command resources
func NewCommand() *starlark.Builtin {
	return starlark.NewBuiltin("command", newCommand)
}

func newCommand(
	thread *starlark.Thread,
	b *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var command starlark.String
	var dependencies *starlark.List

	err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"command", &command,
		"dependencies?", &dependencies,
	)
	if err != nil {
		return nil, err
	}

	// Validate required fields
	if string(command) == "" {
		return nil, fmt.Errorf("command cannot be empty")
	}

	cmd := &Command{
		Command: string(command),
	}

	// Parse dependencies as resource values
	if dependencies != nil {
		deps, err := parseDependencies(dependencies)
		if err != nil {
			return nil, fmt.Errorf("invalid dependencies: %w", err)
		}
		cmd.Dependencies = deps
	}

	return cmd, nil
}

type Command struct {
	Command      string
	Dependencies []starlark.Value
}

func (c *Command) Attr(name string) (starlark.Value, error) {
	switch name {
	case "command":
		return starlark.String(c.Command), nil
	case "dependencies":
		deps := make([]starlark.Value, len(c.Dependencies))
		copy(deps, c.Dependencies)
		return starlark.NewList(deps), nil
	default:
		return nil, nil
	}
}

func (c *Command) Id() string {
	return "command:" + c.Command
}

func (c *Command) AttrNames() []string {
	return []string{"command", "dependencies"}
}

func (c *Command) Type() string {
	return "command"
}

func (c *Command) Freeze() {
	// Freeze dependencies as well
	for _, dep := range c.Dependencies {
		dep.Freeze()
	}
}

func (c *Command) Truth() starlark.Bool {
	return starlark.True
}

func (c *Command) Hash() (uint32, error) {
	return 0, fmt.Errorf("command is unhashable")
}

func (c *Command) String() string {
	return c.Id()
}

func (c *Command) GetDependencies() []starlark.Value {
	deps := make([]starlark.Value, len(c.Dependencies))
	copy(deps, c.Dependencies)
	return deps
}
