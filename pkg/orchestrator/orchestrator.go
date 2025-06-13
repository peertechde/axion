package orchestrator

import (
	"context"
	"fmt"
	"sync"

	"peertech.de/axion/pkg/graph"
	"peertech.de/axion/pkg/report"
	"peertech.de/axion/pkg/resource"
)

// ResourceSpec defines a resource along with its unique identifier and dependencies
type ResourceSpec struct {
	Id           string
	Resource     resource.Resource
	Dependencies []string
}

// Attempt stores the outcome of an attempt to process a single resource.
type Attempt struct {
	Id                string
	Name              string
	Changes           string
	NeedsApply        bool
	EvaluationError   error
	BackupAttempted   bool
	BackedUp          bool
	BackupError       error
	ApplyAttempted    bool
	Applied           bool
	ApplyError        error
	RollbackAttempted bool
	RolledBack        bool
	RollbackError     error
	Skipped           bool
}

func NewOrchestrator(options ...Option) *Orchestrator {
	// Default options
	opts := Options{
		Reporter:    report.EmojiReporter{},
		Concurrency: 1,
	}

	for _, option := range options {
		option(&opts)
	}

	return &Orchestrator{
		options: opts,
		specs:   make(map[string]ResourceSpec),
		g:       graph.New(),
	}
}

// Orchestrator manages the lifecycle of resources and their dependencies. It coordinates
// the evaluation, backup, apply and rollback of resources in the correct dependency
// order.
type Orchestrator struct {
	options Options

	mu    sync.RWMutex            // protects the specs
	specs map[string]ResourceSpec // specs tracked by resource id

	g *graph.Graph
}

// Add registers a new resource with the orchestrator. The resources must have a unique
// identifier and will be validated if it implements the Validatable interface.
//
// Returns an error if:
//   - A resource with the same ID already exists
//   - The resource fails validation (if it implements Validatable)
//   - The resource ID is empty
func (o *Orchestrator) Add(rs ResourceSpec) error {
	if rs.Id == "" {
		return fmt.Errorf("resource ID cannot be empty")
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	if _, exists := o.specs[rs.Id]; exists {
		return fmt.Errorf("duplicate resource spec id: %q", rs.Id)
	}

	// Validate resource if it implements Validatable
	if v, ok := rs.Resource.(resource.Validatable); ok {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("resource validation failed for %q: %w", rs.Id, err)
		}
	}

	o.specs[rs.Id] = rs

	// Add node to the graph
	node := graph.NewNode(rs.Id)
	o.g.AddNode(node)

	return nil
}

// initialize builds the dependency graph.
// Returns an error if any dependency references a unknown resource.
func (o *Orchestrator) initialize() error {
	o.mu.RLock()
	defer o.mu.RUnlock()

	for _, rs := range o.specs {
		id := rs.Id
		for _, dep := range rs.Dependencies {
			// Validate dependency exists before creating edge
			if _, exists := o.specs[dep]; !exists {
				return fmt.Errorf("resource %q depends on unknown resource %q", id, dep)
			}
			err := o.g.AddEdgeByName(dep, id)
			if err != nil {
				return fmt.Errorf("failed wiring dependency from %q to %q: %w", dep, id, err)
			}
		}
	}

	return nil
}

// Run executes the orchestration of all registered resources. Resources are processed in
// dependency order.
//
// The execution flow for each resource is:
//  1. Evaluate - Check if changes are needed and generate diff
//  2. Backup - Create backup if enabled and resource supports it
//  3. Apply - Execute the changes (skipped if planOnly=true)
//  4. Rollback - Revert changes if any subsequent resource fails
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - planOnly: If true, only evaluation and diff generation is performed (dry-run mode)
//
// Returns a Summary containing detailed results of all operations. The Summary.Success
// field indicates overall success/failure.
//
// Behavior notes:
//   - Processing stops on first failure, remaining resources are marked as skipped
//   - On failure, all successfully applied resources are rolled back in reverse order
//   - Context cancellation is respected at resource boundaries
//   - Resources that don't need changes are skipped automatically
//
// NOTE: Currently Run does both live reporting (via o.options.Reporter) and returns a
// Summary. This creates redundant reporting and couples orchestration with presentation.
//
// TODO: Implement alternatives
//   - Remove Reporter dependency, return Summary only (caller handles reporting)
//   - Add Observer pattern for live updates, keep Summary for final state
func (o *Orchestrator) Run(ctx context.Context, planOnly bool) *Summary {
	summary := newSummary()

	if err := o.initialize(); err != nil {
		summary.Error = fmt.Errorf("failed to initialize: %w", err)
		summary.Success = false
		return summary
	}

	nodes, err := o.g.Sort()
	if err != nil {
		summary.Error = fmt.Errorf("dependency resolution failed: %w", err)
		summary.Success = false
		return summary
	}
	summary.TotalCount = len(nodes)

	var failed bool
	applied := make([]*Attempt, 0, len(nodes))

	for _, node := range nodes {
		select {
		case <-ctx.Done():
			summary.Error = ctx.Err()
			summary.Success = false
			return summary
		default:
		}

		rs := o.specs[node.Name]
		res := rs.Resource

		attempt := &Attempt{Id: node.Name, Name: res.Name()}
		summary.Attempts[node.Name] = attempt

		// Skip if previous resource failed
		if failed {
			o.options.Reporter.Skipped(attempt.Id, attempt.Name)
			attempt.Skipped = true
			summary.SkippedCount++
			continue // Continue to mark remaining as skipped
		}

		err = o.evaluate(ctx, attempt, res)
		if err != nil {
			failed = true
			continue // Continue to mark remaining as skipped
		}

		if planOnly || !attempt.NeedsApply {
			continue
		}

		// TODO: Rollback if backup fails?
		// Currently we error out, no rollback attempted here for backup failure.
		err = o.backup(ctx, attempt, res)
		if err != nil {
			failed = true
			continue // Continue to mark remaining as skipped
		}

		err = o.apply(ctx, attempt, res)
		if err != nil {
			failed = true
			continue
		}

		applied = append(applied, attempt)
		summary.AppliedCount++
	}

	if failed && !planOnly {
		n := o.rollback(ctx, applied)
		summary.RollbackCount = n
	}

	summary.Success = !failed
	return summary
}

// evaluate determines the current state of a resource and generates a human-readable diff
// of pending changes.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - r: The resource to evaluate
//
// Returns:
//   - bool: true if the resource needs to be applied
//   - string: human-readable description of changes (empty if no changes needed)
//   - error: any error encountered during evaluation
func (o *Orchestrator) evaluate(ctx context.Context, attempt *Attempt, r resource.Resource) error {
	o.options.Reporter.Evaluate(attempt.Id, attempt.Name)

	needsApply, err := r.Check(ctx)
	if err != nil {
		o.options.Reporter.Fail(attempt.Id, attempt.Name, err)
		attempt.EvaluationError = err
		return err
	}

	attempt.NeedsApply = needsApply
	if attempt.NeedsApply {
		diff, derr := r.Diff(ctx)
		if derr != nil {
			attempt.Changes = "[diff unavailable: " + derr.Error() + "]"
		} else {
			attempt.Changes = diff
		}
		o.options.Reporter.Diff(attempt.Id, attempt.Name, attempt.Changes)
	} else {
		o.options.Reporter.NoChanges(attempt.Id, attempt.Name)
	}

	return nil
}

// apply transitions a resource to the desired state. This method respects context
// cancellation and will return early if the context is cancelled.
//
// Parameters:
// - ctx: Context for cancellation and timeouts
// - r: The resource to apply changes to
//
// Returns any error encountered during the apply operation. A nil return indicates the
// resource was successfully applied.
func (o *Orchestrator) apply(ctx context.Context, attempt *Attempt, r resource.Resource) error {
	o.options.Reporter.Apply(attempt.Id, attempt.Name)

	attempt.ApplyAttempted = true
	err := r.Apply(ctx)
	if err != nil {
		o.options.Reporter.Fail(attempt.Id, attempt.Name, err)
		attempt.ApplyError = err
		return fmt.Errorf("apply failed: %w", err)
	}

	attempt.Applied = true
	o.options.Reporter.Success(attempt.Id, attempt.Name)
	return nil
}

// backup creates a backup of the resource's current state if backup is enabled and the
// resource implements the Backupable interface.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - r: The resource to backup
//
// Returns:
//   - bool: true if a backup was actually created, false otherwise
//   - error: any error encountered during backup
//
// A backup may not be created even without error if: - Backup is disabled in orchestrator
// options - Resource doesn't implement Backupable interface - Resource determines no
// backup is needed (returns false from Backup method)
func (o *Orchestrator) backup(ctx context.Context, attempt *Attempt, r resource.Resource) error {
	if !o.options.BackupEnabled {
		return nil
	}

	b, ok := r.(resource.Backupable)
	if !ok {
		return nil
	}

	attempt.BackupAttempted = true
	backuped, err := b.Backup(ctx)
	if err != nil {
		o.options.Reporter.Fail(attempt.Id, attempt.Name, err)
		attempt.BackupError = err
		return fmt.Errorf("backup failed: %w", err)
	}

	if backuped {
		o.options.Reporter.Backuped(attempt.Id, attempt.Name)
		attempt.BackedUp = true
	}

	return nil
}

// rollback reverts all successfully applied resources to their previous state in reverse
// dependency order.
//
// Rollback is attempted for all resources that were successfully applied, regardless of
// whether individual rollback operations succeed or fail. This ensures maximum recovery
// even if some rollbacks fail.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - applied: Slice of attempts for resources that were successfully applied
//
// Returns the number of resources that were successfully rolled back.
func (o *Orchestrator) rollback(ctx context.Context, applied []*Attempt) int {
	count := 0

	o.options.Reporter.Info("Starting rollback...")
	for i := len(applied) - 1; i >= 0; i-- {
		select {
		case <-ctx.Done():
			o.options.Reporter.Warn(fmt.Sprintf("Rollback interrupted by context cancellation after %d steps", len(applied)-(i+1)))
			return count
		default:
		}

		attempt := applied[i]
		r := o.specs[attempt.Id].Resource

		o.options.Reporter.Rollback(attempt.Id, attempt.Name)
		attempt.RollbackAttempted = true
		err := r.Rollback(ctx)
		if err != nil {
			o.options.Reporter.Fail(attempt.Id, attempt.Name, fmt.Errorf("rollback failed: %w", err))
			attempt.RollbackError = err
		} else {
			attempt.RolledBack = true
			count++
		}
	}

	o.options.Reporter.Info("Rollback finished.")
	return count
}
