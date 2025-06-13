package orchestrator

func newSummary() *Summary {
	return &Summary{
		Attempts: make(map[string]*Attempt),
	}
}

// Summary provides a detailed report of the Apply operation.
type Summary struct {
	Success       bool
	Error         error
	Attempts      map[string]*Attempt // Atttempts keyed by resource Id
	TotalCount    int
	AppliedCount  int
	SkippedCount  int
	RollbackCount int
}
