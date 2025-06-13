package manifest

import (
	"context"

	"peertech.de/axion/pkg/config"
	"peertech.de/axion/pkg/orchestrator"
)

// Loader defines the interface for loading and parsing manifest files. Implementations
// should parse manifest files in their respective formats and return
// orchestrator-compatible resource specifications.
//
// The loader is responsible for:
//   - Reading and parsing manifest files
//   - Processing any templating or variable substitution
//   - Instantiating concrete resource objects
//   - Validating resource configurations
//
// Context usage:
//   - Respect context cancellation during long-running operations
//   - Use context for timeout handling during file I/O operations
//   - Pass context to resource instantiation for timeout control
//
// Parameters:
//   - ctx: Context for cancellation, timeouts, and request-scoped values
//   - cfg: Application configuration containing resource settings, credentials,
//     and environment-specific parameters needed for resource instantiation
//   - path: File system path to the manifest file to be loaded and processed
//
// Returns:
//   - []orchestrator.ResourceSpec: List of resource specifications with unique IDs,
//     concrete resource implementations and resolved dependency references.
//     Resources are returned in declaration order, not dependency order
//     (dependency ordering is handled by the orchestrator).
//   - error: Any error encountered during loading, parsing, validation or
//     resource instantiation.
type Loader interface {
	Load(ctx context.Context, cfg *config.Config, path string) ([]orchestrator.ResourceSpec, error)
}
