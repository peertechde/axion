package starlark_test

import (
	"context"
	"testing"

	"peertech.de/axion/pkg/manifest/starlark"
)

func TestCompleteWorkflow(t *testing.T) {
	src := `
cmd = resources.command(
    command = "date"
)

config = resources.file(
    state = "present",
    path="/etc/app/config.yml",
    mode="0644",
    owner="root"
)

data_dir = resources.directory(
    state = "present",
    path="/var/lib/app",
    mode="0755",
    owner="app",
    dependencies=[config]
)
`

	// Test runtime execution
	rt := starlark.NewRuntime(nil)
	globals, err := rt.Run(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	// Test resource extraction
	resources := rt.GetResources(globals)
	if len(resources) != 3 {
		t.Errorf("Expected 3 resources, got %d", len(resources))
	}
}

func TestTypeAssertion(t *testing.T) {
	src := `
cmd = resources.command(
    command = "date"
)	

config_file = resources.file(
    state = "present",
    path="/etc/myapp/config.yml",
    mode="0644",
    owner="root",
    group="root"
)

app_dir = resources.directory(
    state = "present",
    path="/var/lib/myapp",
    mode="0755",
    owner="myapp",
    group="myapp",
    dependencies=[config_file]
)

log_file = resources.file(
    state = "present",
    path="/var/log/myapp/app.log",
    mode="0644",
    owner="myapp",
    group="myapp",
    dependencies=[config_file, app_dir]
)
`

	r := starlark.NewRuntime(nil)

	dict, err := r.Run(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for _, v := range dict {
		if _, ok := v.(*starlark.Command); ok {
			count++
		}
		if _, ok := v.(*starlark.File); ok {
			count++
		}
		if _, ok := v.(*starlark.Directory); ok {
			count++
		}
	}

	expectedResources := 4
	if count != expectedResources {
		t.Errorf("Expected %d resources, got %d", expectedResources, count)
	}
}
