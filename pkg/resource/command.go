package resource

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"peertech.de/axion/api/models"
	"peertech.de/axion/pkg/config"

	ops_command "peertech.de/axion/api/client/command"
)

func NewCommand(cfg *config.Config, command string, opts ...CommandOption) *Command {
	options := CommandOptions{
		IsConcurrent:      false,
		Timeout:           30 * time.Second,
		ExpectedExitCodes: []int{0},
	}
	for _, opt := range opts {
		opt(&options)
	}

	return &Command{
		cfg:     cfg,
		command: command,
		options: options,
	}
}

type CommandOption func(co *CommandOptions)

// TODO: Implement a Conditioner (Conditional), like Ansible creates/removes
type CommandOptions struct {
	// Whether this command can run concurrently with other resources (default: false)
	IsConcurrent bool

	// Timeout for command execution (default: 30s)
	Timeout time.Duration

	// Expected exit codes (default: [0])
	ExpectedExitCodes []int
}

func WithConcurrent(concurrent bool) CommandOption {
	return func(co *CommandOptions) {
		co.IsConcurrent = concurrent
	}
}

func WithTimeout(timeout time.Duration) CommandOption {
	return func(co *CommandOptions) {
		co.Timeout = timeout
	}
}

func WithExpectedExitCodes(codes ...int) CommandOption {
	return func(co *CommandOptions) {
		co.ExpectedExitCodes = codes
	}
}

// CommandExecutionError represents a command that executed but failed
type CommandExecutionError struct {
	Command  string
	ExitCode int
	Expected []int
	Stdout   string
	Stderr   string
	Duration float32
	Details  string
}

func (e *CommandExecutionError) Error() string {
	return fmt.Sprintf("command '%s' failed with exit code %d (expected %v)",
		e.Command, e.ExitCode, e.Expected)
}

type Command struct {
	cfg *config.Config

	command string
	options CommandOptions
}

func (c *Command) Name() string {
	return "command:" + c.command
}

func (c *Command) Validate() error {
	if c.command == "" {
		return fmt.Errorf("command cannot be empty")
	}

	if c.options.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}

	if len(c.options.ExpectedExitCodes) == 0 {
		return fmt.Errorf("at least one expected exit code must be specified")
	}

	return nil
}

func (c *Command) IsConcurrent() bool {
	return c.options.IsConcurrent
}

func (c *Command) Check(ctx context.Context) (bool, error) {
	return true, nil
}

func (c *Command) Diff(ctx context.Context) (string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "diff -- command: %s\n", c.command)
	fmt.Fprintf(&sb, "+ will execute\n")
	fmt.Fprintf(&sb, "  timeout: %v\n", c.options.Timeout)
	fmt.Fprintf(&sb, "  expected_exit_codes: %v\n", c.options.ExpectedExitCodes)

	return sb.String(), nil
}

func (c *Command) Apply(ctx context.Context) error {
	r := &models.CommandRequest{
		Command:           c.command,
		ExpectedExitCodes: make([]int64, len(c.options.ExpectedExitCodes)),
	}

	// Convert expected exit codes
	for i, code := range c.options.ExpectedExitCodes {
		r.ExpectedExitCodes[i] = int64(code)
	}

	// Execute command via API
	params := ops_command.NewExecuteCommandParamsWithContext(ctx)
	params.Command = r

	resp, err := c.cfg.Client.Command.ExecuteCommand(params)
	if err != nil {
		// Only handle actual HTTP/API errors here (network, timeout, server errors)
		if payload := getErrorPayload(err); payload != nil {
			switch payload.Code {
			case http.StatusBadRequest:
				return &APIError{
					Code:    payload.Code,
					Message: fmt.Sprintf("Invalid command request '%s': %s", c.command, payload.Message),
				}
			case http.StatusRequestTimeout:
				return &APIError{
					Code:    payload.Code,
					Message: fmt.Sprintf("Command timed out after %v: %s", c.options.Timeout, c.command),
				}
			case http.StatusInternalServerError:
				return &APIError{
					Code:    payload.Code,
					Message: fmt.Sprintf("Server error executing command '%s': %s", c.command, payload.Message),
				}
			default:
				return &APIError{
					Code:    payload.Code,
					Message: fmt.Sprintf("Failed to execute command '%s': %s", c.command, payload.Message),
				}
			}
		}
		return fmt.Errorf("failed to execute command '%s': %w", c.command, err)
	}

	if resp.Payload == nil {
		return fmt.Errorf("received empty response for command: %s", c.command)
	}

	if !resp.Payload.Success {
		// Build detailed error message with execution details
		var details strings.Builder
		fmt.Fprintf(&details, "Command: %s\n", c.command)
		fmt.Fprintf(&details, "Exit Code: %d\n", resp.Payload.ExitCode)
		fmt.Fprintf(&details, "Expected Exit Codes: %v\n", c.options.ExpectedExitCodes)

		if resp.Payload.Stdout != "" {
			fmt.Fprintf(&details, "Stdout:\n%s\n", resp.Payload.Stdout)
		}

		if resp.Payload.Stderr != "" {
			fmt.Fprintf(&details, "Stderr:\n%s\n", resp.Payload.Stderr)
		}

		return &CommandExecutionError{
			Command:  c.command,
			ExitCode: int(resp.Payload.ExitCode),
			Expected: c.options.ExpectedExitCodes,
			Stdout:   resp.Payload.Stdout,
			Stderr:   resp.Payload.Stderr,
			Details:  details.String(),
		}
	}

	return nil
}

func (c *Command) Backup(ctx context.Context) (bool, error) {
	return false, nil
}

func (c *Command) Rollback(ctx context.Context) error {
	return nil
}
