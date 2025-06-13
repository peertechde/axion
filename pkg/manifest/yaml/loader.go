package yaml

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"

	"gopkg.in/yaml.v3"

	"peertech.de/axion/pkg/config"
	"peertech.de/axion/pkg/orchestrator"
	"peertech.de/axion/pkg/resource"
)

// Manifest represents the complete YAML manifest structure containing variables for
// templating and a list of resources to be managed.
type Manifest struct {
	Variables map[string]any `yaml:"variables" json:"variables"`
	Resources []Resource     `yaml:"resources" json:"resources"`
}

// Resource represents a single resource definition in the manifest.
type Resource struct {
	Id           string         `yaml:"id" json:"id"`
	Type         string         `yaml:"type" json:"type"`
	State        string         `yaml:"state" json:"state"`
	Properties   map[string]any `yaml:"properties" json:"properties"`
	Dependencies []string       `yaml:"dependencies" json:"dependencies"`
}

// Loader implements the manifest.Loader interface for YAML-based manifests
type Loader struct{}

// Load executes a Starlark script and extracts resource specifications
func (l *Loader) Load(ctx context.Context, cfg *config.Config, path string) ([]orchestrator.ResourceSpec, error) {
	m, err := load(path)
	if err != nil {
		return nil, fmt.Errorf("manifest load error [%s]: %w", path, err)
	}

	// Instantiate all resources
	resources := make(map[string]resource.Resource, len(m.Resources))
	for _, spec := range m.Resources {
		r, err := instantiateResource(cfg, spec)
		if err != nil {
			return nil, fmt.Errorf("manifest error: %s", err.Error())
		}
		resources[spec.Id] = r
	}

	// Build orchestrator resources
	var out []orchestrator.ResourceSpec
	for _, spec := range m.Resources {
		r := resources[spec.Id]
		out = append(out, orchestrator.ResourceSpec{
			Id:           spec.Id,
			Resource:     r,
			Dependencies: spec.Dependencies,
		})
	}

	return out, nil
}

// load reads and processes a YAML manifest file with template variable substitution.
//
// Template syntax uses {{ }} delimiters for variable substitution.
//
// Parameters:
//   - path: File system path to the YAML manifest file
//
// Returns:
//   - *Manifest: Parsed manifest with all variables substituted
//   - error: Any error from file reading, template parsing, or YAML parsing
func load(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest file error: %w", err)
	}

	// Initial parsing to extract variables
	var preliminary struct {
		Variables map[string]any `yaml:"variables"`
	}
	if err := yaml.Unmarshal(raw, &preliminary); err != nil {
		return nil, fmt.Errorf("parse variables error: %w", err)
	}

	// Substitute variables
	tmpl, err := template.New("manifest").Delims("{{", "}}").Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("template parse error: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, preliminary.Variables); err != nil {
		return nil, fmt.Errorf("template execution error: %w", err)
	}

	var m Manifest
	if err := yaml.Unmarshal(buf.Bytes(), &m); err != nil {
		return nil, fmt.Errorf("final manifest parse error: %w", err)
	}

	return &m, nil
}

// instantiateResource creates a concrete resource object from a resource specification.
// The function maps resource types to their corresponding implementations and validates
// the resulting resource if it implements the Validatable interface.
//
// Currently supported resource types:
//   - "file": File system resources with path, mode, owner, and group properties
//
// Parameters:
//   - cfg: Application configuration needed for resource construction
//   - res: Resource specification from the manifest
//
// Returns:
//   - resource.Resource: Concrete resource implementation
//   - error: Validation error or unsupported resource type error
func instantiateResource(cfg *config.Config, res Resource) (resource.Resource, error) {
	var r resource.Resource

	switch res.Type {
	case "command":
		props := res.Properties
		r = resource.NewCommand(
			cfg,
			toString(props["command"]),
		)
	case "file":
		props := res.Properties
		r = resource.NewFile(
			cfg,
			resource.State(res.State),
			toString(props["path"]),
			optString(props["mode"]),
			optString(props["owner"]),
			optString(props["group"]),
		)
	case "directory":
		props := res.Properties
		r = resource.NewDirectory(
			cfg,
			resource.State(res.State),
			toString(props["path"]),
			optString(props["mode"]),
			optString(props["owner"]),
			optString(props["group"]),
		)
	default:
		return nil, fmt.Errorf("unsupported resource type %q", res.Type)
	}

	if v, ok := r.(resource.Validatable); ok {
		if err := v.Validate(); err != nil {
			return nil, fmt.Errorf(
				"invalid %q resource (id: %s): %s", res.Type, res.Id, err.Error(),
			)
		}
	}

	return r, nil
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func optString(v any) *string {
	if v == nil {
		return nil
	}
	s := toString(v)
	return &s
}
