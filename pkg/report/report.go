package report

import (
	"fmt"
	"time"
)

type Reporter interface {
	// Info logs general informational messages to the user
	Info(msg string)

	// Warn logs warnings about misconfiguration or recoverable issues.
	Warn(msg string)

	// Error logs non-fatal errors
	Error(msg string)

	// Evaluate reports the start of resource evaluation
	Evaluate(id, name string)

	// NoChanges reports a resource that doesn't need changes after evaluation
	NoChanges(id, name string)

	// Skipped reports a resource that was skipped due to previous failures
	Skipped(id, name string)

	// Diff reports that a resource has differences
	Diff(id, name, diff string)

	// Apply reports the start of a resource apply
	Apply(id, name string)

	// Backuped reports a successfuly backup of a resource
	Backuped(id, name string)

	// Rollback reports the start of a rollback for a resource
	Rollback(id, name string)

	// Success reports successful application
	Success(id, name string)

	// Fail reports a failure
	Fail(id, name string, err error)
}

func timestamp() string {
	return time.Now().Format(time.TimeOnly)
}

func display(id, name string) string {
	if id != "" && id != name {
		return fmt.Sprintf("%s (%s)", name, id)
	}
	return name
}

type EmojiReporter struct{}

func (r EmojiReporter) Info(msg string) {
	fmt.Printf("%s 📢 %s\n", timestamp(), msg)
}

func (r EmojiReporter) Warn(msg string) {
	fmt.Printf("%s ⚠️  %s\n", timestamp(), msg)
}

func (r EmojiReporter) Error(msg string) {
	fmt.Printf("%s ❌ %s\n", timestamp(), msg)
}

func (r EmojiReporter) Evaluate(id, name string) {
	fmt.Printf("%s 🔍 Evaluating: %s\n", timestamp(), display(id, name))
}

func (r EmojiReporter) NoChanges(id, name string) {
	fmt.Printf("%s ✨ No changes needed: %s\n", timestamp(), display(id, name))
}

func (r EmojiReporter) Skipped(id, name string) {
	fmt.Printf("%s ⏭️ Skipped due to failure: %s\n", timestamp(), display(id, name))
}

func (r EmojiReporter) Diff(id, name, diff string) {
	fmt.Printf("%s 📄 Diff for %s:\n%s\n", timestamp(), display(id, name), diff)
}

func (r EmojiReporter) Apply(id, name string) {
	fmt.Printf("%s 🔧 Applying: %s\n", timestamp(), display(id, name))
}

func (r EmojiReporter) Backuped(id, name string) {
	fmt.Printf("%s 💾 Backed up: %s\n", timestamp(), display(id, name))
}

func (r EmojiReporter) Rollback(id, name string) {
	fmt.Printf("%s ↩️ Rolling back: %s\n", timestamp(), display(id, name))
}

func (r EmojiReporter) Success(id, name string) {
	fmt.Printf("%s ✅ Success: %s\n", timestamp(), display(id, name))
}

func (r EmojiReporter) Fail(id, name string, err error) {
	fmt.Printf("%s ❌ Failed: %s — %s\n", timestamp(), display(id, name), err)
}

type PlainReporter struct{}

func (r PlainReporter) Info(msg string) {
	fmt.Printf("%s Info: %s\n", timestamp(), msg)
}

func (r PlainReporter) Warn(msg string) {
	fmt.Printf("%s Warning: %s\n", timestamp(), msg)
}

func (r PlainReporter) Error(msg string) {
	fmt.Printf("%s Error: %s\n", timestamp(), msg)
}

func (r PlainReporter) Evaluate(id, name string) {
	fmt.Printf("%s Evaluating: %s\n", timestamp(), display(id, name))
}

func (r PlainReporter) NoChanges(id, name string) {
	fmt.Printf("%s No changes needed: %s\n", timestamp(), display(id, name))
}

func (r PlainReporter) Skipped(id, name string) {
	fmt.Printf("%s Skipped due to failure: %s\n", timestamp(), display(id, name))
}

func (r PlainReporter) Diff(id, name, diff string) {
	fmt.Printf("%s Diff for %s:\n%s\n", timestamp(), display(id, name), diff)
}

func (r PlainReporter) Apply(id, name string) {
	fmt.Printf("%s Applying: %s\n", timestamp(), display(id, name))
}

func (r PlainReporter) Backuped(id, name string) {
	fmt.Printf("%s Backed up: %s\n", timestamp(), display(id, name))
}

func (r PlainReporter) Rollback(id, name string) {
	fmt.Printf("%s Rolling back: %s\n", timestamp(), display(id, name))
}

func (r PlainReporter) Success(id, name string) {
	fmt.Printf("%s Success: %s\n", timestamp(), display(id, name))
}

func (r PlainReporter) Fail(id, name string, err error) {
	fmt.Printf("%s Failed: %s — %v\n", timestamp(), display(id, name), err)
}

type NilReporter struct{}

func (r NilReporter) Info(msg string)                 {}
func (r NilReporter) Warn(msg string)                 {}
func (r NilReporter) Error(msg string)                {}
func (r NilReporter) Evaluate(id, name string)        {}
func (r NilReporter) NoChanges(id, name string)       {}
func (r NilReporter) Skipped(id, name string)         {}
func (r NilReporter) Diff(id, name, diff string)      {}
func (r NilReporter) Apply(id, name string)           {}
func (r NilReporter) Backuped(id, name string)        {}
func (r NilReporter) Rollback(id, name string)        {}
func (r NilReporter) Success(id, name string)         {}
func (r NilReporter) Fail(id, name string, err error) {}
