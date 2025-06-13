package orchestrator

import "peertech.de/axion/pkg/report"

type Option = func(*Options)

type Options struct {
	Reporter      report.Reporter
	DryRun        bool
	BackupEnabled bool
	Concurrency   int
}

func WithReporter(r report.Reporter) Option {
	return func(o *Options) {
		o.Reporter = r
	}
}

func WithDryRun() Option {
	return func(o *Options) {
		o.DryRun = true
	}
}

func WithEnableBackups() Option {
	return func(o *Options) {
		o.BackupEnabled = true
	}
}

func WithConcurrency(n int) Option {
	return func(o *Options) {
		o.Concurrency = n
	}
}
