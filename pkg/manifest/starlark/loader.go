package starlark

import (
	"os"
	"path/filepath"
)

func load(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	body, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
