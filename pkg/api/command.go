package api

import (
	"context"
	"errors"
	"net/http"
	"os/exec"
	"strings"
	"syscall"

	"github.com/go-openapi/runtime/middleware"
	"github.com/google/shlex"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"peertech.de/axion/api/models"
	ops_command "peertech.de/axion/api/restapi/operations/command"
)

func (api *API) handleCommand(params ops_command.ExecuteCommandParams) middleware.Responder {
	scopedLog := log.With().
		Str("handler", "handleCommand").
		Str("command", params.Command.Command).
		Logger()

	if params.Command.Command == "" {
		return ops_command.NewExecuteCommandBadRequest().
			WithPayload(newAPIError(http.StatusBadRequest, WithMessage("Command cannot be empty")))
	}

	expectedExitCodes := []int{0}
	if len(params.Command.ExpectedExitCodes) > 0 {
		expectedExitCodes = make([]int, len(params.Command.ExpectedExitCodes))
		for i, code := range params.Command.ExpectedExitCodes {
			expectedExitCodes[i] = int(code)
		}
	}

	// Execute command
	result, err := api.executeCommand(params.HTTPRequest.Context(), scopedLog, params.Command)
	if err != nil {
		var oe *OpError
		if errors.As(err, &oe) {
			switch oe.Code {
			case http.StatusBadRequest:
				return ops_command.NewExecuteCommandBadRequest().
					WithPayload(newAPIError(oe.Code, WithMessage(oe.Msg)))
			case http.StatusRequestTimeout:
				return ops_command.NewExecuteCommandRequestTimeout().
					WithPayload(newAPIError(oe.Code, WithMessage(oe.Msg)))
			case http.StatusInternalServerError:
				scopedLog.Error().Err(oe.Cause).Msg(oe.Msg)
				return ops_command.NewExecuteCommandInternalServerError().
					WithPayload(newAPIError(oe.Code, WithMessage(oe.Msg)))
			default:
				scopedLog.Error().Err(oe.Cause).Msg(oe.Msg)
				return ops_command.NewExecuteCommandInternalServerError().
					WithPayload(newAPIError(http.StatusInternalServerError, WithMessage("Command execution failed")))
			}
		} else {
			// Fallback for unexpected error types
			scopedLog.Error().Err(err).Msg("Unexpected error during command execution")
			return ops_command.NewExecuteCommandInternalServerError().
				WithPayload(newAPIError(http.StatusInternalServerError, WithMessage("Command execution failed")))
		}
	}

	// Check if exit code is expected
	success := false
	for _, expectedCode := range expectedExitCodes {
		if result.ExitCode == int64(expectedCode) {
			success = true
			break
		}
	}
	result.Success = success

	if success {
		scopedLog.Debug().
			Int64("exit_code", result.ExitCode).
			Msg("Command executed successfully")
	} else {
		scopedLog.Debug().
			Int64("exit_code", result.ExitCode).
			Ints("expected_codes", expectedExitCodes).
			Msg("Command executed but exit code was not expected")
	}

	return ops_command.NewExecuteCommandOK().WithPayload(result)
}

func (api *API) executeCommand(ctx context.Context, scopedLog zerolog.Logger, r *models.CommandRequest) (*models.CommandResponse, error) {
	// Split command string
	parts, err := shlex.Split(r.Command)
	if err != nil {
		return nil, newOpError(http.StatusBadRequest, "Invalid command syntax", err)
	}
	if len(parts) == 0 {
		return nil, newOpError(http.StatusBadRequest, "Empty command", nil)
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)

	// Capture output
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	// Determine exit code
	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			}
		} else if ctx.Err() == context.DeadlineExceeded {
			return nil, newOpError(http.StatusRequestTimeout, "Command execution timed out", err)
		} else {
			// Other execution errors (command not found, permission denied, etc.)
			return nil, newOpError(http.StatusInternalServerError, "Command execution failed", err)
		}
	}

	result := &models.CommandResponse{
		ExitCode: int64(exitCode),
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}

	scopedLog.Debug().
		Int64("exit_code", result.ExitCode).
		Int("stdout_len", len(result.Stdout)).
		Int("stderr_len", len(result.Stderr)).
		Msg("Command execution completed")

	return result, nil
}
