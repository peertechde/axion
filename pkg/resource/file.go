package resource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	ops_content "peertech.de/axion/api/client/content"
	ops_files "peertech.de/axion/api/client/files"
	"peertech.de/axion/api/models"
	"peertech.de/axion/pkg/config"
	"peertech.de/axion/pkg/pointer"
)

func NewFile(cfg *config.Config, state State, path string, mode, owner, group *string) *File {
	return &File{
		cfg:               cfg,
		desiredState:      state,
		path:              path,
		desiredProperties: &fileProperties{Mode: mode, Owner: owner, Group: group},
	}
}

type fileProperties struct {
	Mode  *string
	Owner *string
	Group *string
}

type File struct {
	cfg *config.Config

	desiredState      State
	path              string
	desiredProperties *fileProperties

	currentState      State
	currentProperties *models.FileProperties
	etag              string

	// Track the operation we made
	lastOperation Operation
}

func (f *File) Name() string {
	return "file:" + f.path
}

func (f *File) Validate() error {
	switch f.desiredState {
	case StateAbsent, StatePresent:
	default:
		return fmt.Errorf("invalid desired state for file: %q", f.desiredState)
	}

	if f.path == "" {
		return fmt.Errorf("file path cannot be empty")
	}

	if f.desiredProperties.Mode != nil && !isValidFileMode(*f.desiredProperties.Mode) {
		return fmt.Errorf("invalid file mode: %q", *f.desiredProperties.Mode)
	}

	return nil
}

func isValidFileMode(mode string) bool {
	_, err := strconv.ParseUint(mode, 8, 32) // octal mode
	return err == nil
}

func (f *File) IsConcurrent() bool {
	return true
}

func (f *File) Check(ctx context.Context) (bool, error) {
	params := ops_files.NewGetFilePropertiesParamsWithContext(ctx)
	params.Path = f.path

	resp, err := f.cfg.Client.Files.GetFileProperties(params)
	if err != nil {
		if fileNotFound(err) {
			f.currentState = StateAbsent
			f.currentProperties = nil
			f.etag = ""

			// If desired state is absent, no action needed
			// If desired state is present, action needed
			return f.desiredState == StatePresent, nil
		}
		if payload := getErrorPayload(err); payload != nil {
			return false, &APIError{Code: payload.Code, Message: payload.Message}
		}

		return false, fmt.Errorf("failed to check file")
	}

	if resp.Payload == nil {
		return false, fmt.Errorf("received empty payload")
	}

	f.currentState = StatePresent
	f.currentProperties = resp.Payload
	f.etag = resp.ETag

	// File exists but should be absent, needs action
	if f.desiredState == StateAbsent {
		return true, nil
	}

	// Check if all desired properties match current properties
	return !f.propertiesMatch(), nil
}

// propertiesMatch checks if current properties match desired properties
func (f *File) propertiesMatch() bool {
	if f.currentProperties == nil {
		return false
	}

	if f.desiredProperties.Mode != nil && *f.desiredProperties.Mode != f.currentProperties.Mode {
		return false
	}
	if f.desiredProperties.Owner != nil && *f.desiredProperties.Owner != f.currentProperties.Owner {
		return false
	}
	if f.desiredProperties.Group != nil && *f.desiredProperties.Group != f.currentProperties.Group {
		return false
	}

	return true
}

func (f *File) Diff(ctx context.Context) (string, error) {
	switch {
	case f.desiredState == StateAbsent && f.currentState == StatePresent:
		return fmt.Sprintf("diff -- file: %s\n- present (file will be deleted)\n", f.path), nil
	case f.desiredState == StatePresent && f.currentState == StateAbsent:
		return fmt.Sprintf("diff -- file: %s\n+ present (file will be created)\n", f.path), nil
	}

	if f.currentProperties == nil {
		return "", fmt.Errorf("no current state available for diff")
	}
	if f.desiredProperties == nil {
		return "", fmt.Errorf("no desired state available for diff")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "diff -- file: %s\n", f.path)

	compare := func(name string, desired *string, actual string) {
		if desired != nil && *desired != actual {
			fmt.Fprintf(&sb, "- %s: %q\n+ %s: %q\n", name, actual, name, *desired)
		}
	}

	compare("mode", f.desiredProperties.Mode, f.currentProperties.Mode)
	compare("owner", f.desiredProperties.Owner, f.currentProperties.Owner)
	compare("group", f.desiredProperties.Group, f.currentProperties.Group)

	if sb.Len() == 0 {
		return "", nil
	}

	return sb.String(), nil
}

func (f *File) Apply(ctx context.Context) error {
	f.lastOperation = OperationNone

	if f.desiredState == StateAbsent {
		if f.currentState == f.desiredState {
			return nil
		}

		params := ops_files.NewDeleteFileParamsWithContext(ctx)
		params.Path = f.path
		if f.etag != "" {
			params.SetIfMatch(pointer.To(f.etag))
		}

		_, err := f.cfg.Client.Files.DeleteFile(params)
		if err != nil {
			if payload := getErrorPayload(err); payload != nil {
				return &APIError{Code: payload.Code, Message: payload.Message}
			}

			return fmt.Errorf("failed to apply file: %w", err)
		}

		f.lastOperation = OperationDelete
		return nil
	}

	props := &models.FileProperties{}
	if f.desiredProperties.Mode != nil {
		props.Mode = *f.desiredProperties.Mode
	}
	if f.desiredProperties.Owner != nil {
		props.Owner = *f.desiredProperties.Owner
	}
	if f.desiredProperties.Group != nil {
		props.Group = *f.desiredProperties.Group
	}

	params := ops_files.NewPutFileParamsWithContext(ctx)
	params.Path = f.path
	params.Properties = props

	// Existing file, enforce ETag
	if f.currentProperties != nil && f.etag != "" {
		params.SetIfMatch(pointer.To(f.etag))
	}

	created, noContent, err := f.cfg.Client.Files.PutFile(params)
	if err != nil {
		if payload := getErrorPayload(err); payload != nil {
			return &APIError{Code: payload.Code, Message: payload.Message}
		}

		return fmt.Errorf("failed to apply file: %w", err)
	}

	switch {
	case created != nil:
		f.lastOperation = OperationCreate
		f.etag = created.ETag
	case noContent != nil:
		f.lastOperation = OperationUpdate
		f.etag = noContent.ETag
	default:
		return fmt.Errorf("unexpected nil response")
	}

	return nil
}

func (f *File) Backup(ctx context.Context) (bool, error) {
	// Only backup if file exists
	if f.currentState != StatePresent {
		return false, nil
	}

	// If desired state is absent, backup content for full restore
	if f.desiredState == StateAbsent {
		return f.backup(ctx)
	}

	// If desired state is present and properties are changing, backup current properties
	// (f.currentProperties is already stored).
	return false, nil
}

func (f *File) backup(ctx context.Context) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(f.backupPath()), 0755); err != nil {
		return false, err
	}

	fd, err := os.Create(f.backupPath())
	if err != nil {
		return false, err
	}
	defer fd.Close()

	params := ops_content.NewDownloadParamsWithContext(ctx)
	params.Path = f.path
	params.Recursive = pointer.To(false)

	_, err = f.cfg.Client.Content.Download(params, fd)
	if err != nil {
		// Clean up backup file on error
		os.Remove(f.backupPath())

		if payload := getErrorPayload(err); payload != nil {
			return false, &APIError{Code: payload.Code, Message: payload.Message}
		}

		return false, fmt.Errorf("failed to backup file: %w", err)
	}

	return true, nil
}

func (f *File) Rollback(ctx context.Context) error {
	switch f.lastOperation {
	case OperationNone:
		return nil
	case OperationCreate:
		params := ops_files.NewDeleteFileParamsWithContext(ctx)
		params.Path = f.path
		params.SetIfMatch(pointer.To(f.etag))

		_, err := f.cfg.Client.Files.DeleteFile(params)
		if err != nil {
			if payload := getErrorPayload(err); payload != nil {
				return &APIError{Code: payload.Code, Message: payload.Message}
			}

			return fmt.Errorf("failed to delete file: %w", err)
		}
		return err
	case OperationUpdate:
		return f.rollbackProperties(ctx)
	case OperationDelete:
		return f.restoreFromBackup(ctx)
	}

	return nil
}

func (f *File) rollbackProperties(ctx context.Context) error {
	if f.currentProperties == nil {
		return fmt.Errorf("no properties to rollback")
	}

	props := &models.FileProperties{
		Mode:  f.currentProperties.Mode,
		Owner: f.currentProperties.Owner,
		Group: f.currentProperties.Group,
	}

	params := ops_files.NewPutFileParamsWithContext(ctx)
	params.Path = f.path
	params.Properties = props

	if f.etag != "" {
		params.SetIfMatch(pointer.To(f.etag))
	}

	_, _, err := f.cfg.Client.Files.PutFile(params)
	if err != nil {
		if payload := getErrorPayload(err); payload != nil {
			return &APIError{Code: payload.Code, Message: payload.Message}
		}

		return fmt.Errorf("failed to put file: %w", err)
	}

	return nil
}

func (f *File) backupPath() string {
	safe := strings.ReplaceAll(strings.TrimPrefix(f.path, "/"), "/", "-")
	return filepath.Join(f.cfg.BackupDir, safe+".tar.gz")
}

func (f *File) restoreFromBackup(ctx context.Context) error {
	// Check if backup file exists
	if _, err := os.Stat(f.backupPath()); os.IsNotExist(err) {
		return fmt.Errorf("no backup file found at %s", f.backupPath())
	}

	fd, err := os.Open(f.backupPath())
	if err != nil {
		return fmt.Errorf("failed to open backup: %w", err)
	}
	defer fd.Close()

	params := ops_content.NewUploadParamsWithContext(ctx)
	params.Path = f.path
	params.Recursive = pointer.To(false)
	params.Content = fd

	_, _, err = f.cfg.Client.Content.Upload(params)
	if err != nil {
		if payload := getErrorPayload(err); payload != nil {
			return &APIError{Code: payload.Code, Message: payload.Message}
		}
		return fmt.Errorf("failed to restore file from backup: %w", err)
	}

	// TODO: Clean up backup file after successful restore

	return nil
}
