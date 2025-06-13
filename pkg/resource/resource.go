package resource

import "context"

// State represents the state of a resource.
type State string

const (
	// StateUnknown indicates the resource state cannot be determined
	StateUnknown State = "unknown"
	// StateAbsent indicates the resource does not exist or is not configured
	StateAbsent State = "absent"
	// StatePresent indicates the resource exists and is properly configured
	StatePresent State = "present"
)

// Operation represents what operation was performed during Apply
type Operation int

const (
	OperationNone Operation = iota
	OperationCreate
	OperationUpdate
	OperationDelete
)

// Resource defines the core interface for all manageable resources in the system.
//
// Implementations must be safe for concurrent use if IsConcurrent returns true.
type Resource interface {
	// Name returns a human-readable identifier for the resource.
	Name() string

	// IsConcurrent indicates whether this resource can be safely processed in parallel
	// with other resources.
	IsConcurrent() bool

	// Check determines whether the resource needs to be applied by comparing the current
	// system state with the desired configuration.
	//
	// Returns:
	//   - true: resource needs to be applied (current state doesn't match desired)
	//   - false: resource is already in the desired state, no action needed
	//   - error: if the check operation failed
	//
	// This method should be idempotent and have no side effects.
	Check(ctx context.Context) (bool, error)

	// Diff returns a human-readable representation of the differences between the current
	// and desired state of the resource. It is used during dry-run or plan operations to
	// preview changes before applying them.
	//
	// The output format is unified across resource types and typically uses a Git-style
	// diff format (lines prefixed with "+" or "-").
	Diff(ctx context.Context) (string, error)

	// Apply executes the necessary operations to transition the resource from its current
	// state to the desired state.
	//
	// This method should be idempotent - calling it multiple times with the same desired
	// state should produce the same result without harmful side effects.
	//
	// If an error occurs during application, the resource may be left in a partially
	// configured state. Use Rollback to attempt recovery.
	Apply(ctx context.Context) error

	// Rollback attempts to restore the resource to its previous state before the last
	// Apply operation.
	//
	// This is a best-effort operation and may not always be possible (e.g., if backup
	// data is unavailable or if the rollback operation itself fails). Implementations
	// should document their rollback capabilities and limitations.
	Rollback(ctx context.Context) error
}

// Validatable extends Resource with configuration validation capabilities. Resources
// implementing this interface can validate their configuration before attempting any
// state changes.
type Validatable interface {
	// Validate checks the resource configuration for errors, missing required fields, or
	// invalid values.
	//
	// Returns nil if the configuration is valid, or a descriptive error explaining what
	// needs to be corrected.
	Validate() error
}

// Backupable extends Resource with backup capabilities. Resources implementing this
// interface can create backups of their current state before making changes, enabling
// more reliable rollbacks.
type Backupable interface {
	// Backup creates a snapshot of the resource's current state that can be used for
	// rollback operations.
	//
	// The backup should capture all necessary information to restore the resource to its
	// exact current configuration. Backup data is typically stored in a format that can
	// be consumed by the Rollback method.
	//
	// Returns:
	//   - bool: true if a backup was actually created, false if no backup
	//     was needed (e.g., resource is in initial state, backup already exists,
	//     or resource doesn't require backup for the current operation)
	//   - error: non-nil if the backup operation failed
	//
	// This method is usually called automatically before Apply operations on resources
	// that support backup/restore functionality.
	Backup(ctx context.Context) (bool, error)
}
