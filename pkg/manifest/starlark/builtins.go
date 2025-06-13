package starlark

import (
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

var MakeStruct = starlark.NewBuiltin("struct", starlarkstruct.Make)
