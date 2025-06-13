package resource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	ops_content "peertech.de/axion/api/client/content"
	ops_directories "peertech.de/axion/api/client/directories"
	"peertech.de/axion/api/models"
	"peertech.de/axion/pkg/config"
	"peertech.de/axion/pkg/pointer"
)

func NewDirectory(cfg *config.Config, state State, path string, mode, owner, group *string) *Directory {
	return &Directory{
		cfg:               cfg,
		desiredState:      state,
		path:              path,
		desiredProperties: &directoryProperties{Mode: mode, Owner: owner, Group: group},
	}
}

type directoryProperties struct {
	Mode  *string
	Owner *string
	Group *string
}

type Directory struct {
	cfg *config.Config

	desiredState      State
	path              string
	desiredProperties *directoryProperties

	currentState      State
	currentProperties *models.DirectoryProperties
	etag              string

	// Track the operation we made
	lastOperation Operation
}

func (d *Directory) Name() string {
	return "directory:" + d.path
}

func (d *Directory) Validate() error {
	switch d.desiredState {
	case StateAbsent, StatePresent:
	default:
		return fmt.Errorf("invalid desired state for directory: %q", d.desiredState)
	}

	if d.path == "" {
		return fmt.Errorf("directory path cannot be empty")
	}

	if d.desiredProperties.Mode != nil && !isValidDirectoryMode(*d.desiredProperties.Mode) {
		return fmt.Errorf("invalid directory mode: %q", *d.desiredProperties.Mode)
	}

	return nil
}

func isValidDirectoryMode(mode string) bool {
	_, err := strconv.ParseUint(mode, 8, 32) // octal mode
	return err == nil
}

func (d *Directory) IsConcurrent() bool {
	return true
}

func (d *Directory) Check(ctx context.Context) (bool, error) {
	params := ops_directories.NewGetDirectoryPropertiesParamsWithContext(ctx)
	params.Path = d.path

	resp, err := d.cfg.Client.Directories.GetDirectoryProperties(params)
	if err != nil {
		if directoryNotFound(err) {
			d.currentState = StateAbsent
			d.currentProperties = nil
			d.etag = ""

			// If desired state is absent, no action needed
			// If desired state is present, action needed
			return d.desiredState == StatePresent, nil
		}
		if payload := getErrorPayload(err); payload != nil {
			return false, &APIError{Code: payload.Code, Message: payload.Message}
		}

		return false, fmt.Errorf("failed to check file")
	}

	if resp.Payload == nil {
		return false, fmt.Errorf("received empty payload")
	}

	d.currentState = StatePresent
	d.currentProperties = resp.Payload
	d.etag = resp.ETag

	// Directory exists but should be absent, needs action
	if d.desiredState == StateAbsent {
		return true, nil
	}

	// Check if all desired properties match current properties
	return !d.propertiesMatch(), nil
}

// propertiesMatch checks if current properties match desired properties
func (d *Directory) propertiesMatch() bool {
	if d.currentProperties == nil {
		return false
	}

	if d.desiredProperties.Mode != nil && *d.desiredProperties.Mode != d.currentProperties.Mode {
		return false
	}
	if d.desiredProperties.Owner != nil && *d.desiredProperties.Owner != d.currentProperties.Owner {
		return false
	}
	if d.desiredProperties.Group != nil && *d.desiredProperties.Group != d.currentProperties.Group {
		return false
	}

	return true
}

func (d *Directory) Diff(ctx context.Context) (string, error) {
	switch {
	case d.desiredState == StateAbsent && d.currentState == StatePresent:
		return fmt.Sprintf("diff -- file: %s\n- present (file will be deleted)\n", d.path), nil
	case d.desiredState == StatePresent && d.currentState == StateAbsent:
		return fmt.Sprintf("diff -- file: %s\n+ present (file will be created)\n", d.path), nil
	}

	if d.currentProperties == nil {
		return "", fmt.Errorf("no current state available for diff")
	}
	if d.desiredProperties == nil {
		return "", fmt.Errorf("no desired state available for diff")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "diff -- file: %s\n", d.path)

	compare := func(name string, desired *string, actual string) {
		if desired != nil && *desired != actual {
			fmt.Fprintf(&sb, "- %s: %q\n+ %s: %q\n", name, actual, name, *desired)
		}
	}

	compare("mode", d.desiredProperties.Mode, d.currentProperties.Mode)
	compare("owner", d.desiredProperties.Owner, d.currentProperties.Owner)
	compare("group", d.desiredProperties.Group, d.currentProperties.Group)

	if sb.Len() == 0 {
		return "", nil
	}

	return sb.String(), nil
}

func (d *Directory) Apply(ctx context.Context) error {
	d.lastOperation = OperationNone

	if d.desiredState == StateAbsent {
		if d.currentState == d.desiredState {
			return nil
		}

		params := ops_directories.NewDeleteDirectoryParamsWithContext(ctx)
		params.Path = d.path
		if d.etag != "" {
			params.SetIfMatch(pointer.To(d.etag))
		}

		_, err := d.cfg.Client.Directories.DeleteDirectory(params)
		if err != nil {
			if payload := getErrorPayload(err); payload != nil {
				return &APIError{Code: payload.Code, Message: payload.Message}
			}

			return fmt.Errorf("failed to apply file: %w", err)
		}

		d.lastOperation = OperationDelete
		return nil
	}

	props := &models.DirectoryProperties{}
	if d.desiredProperties.Mode != nil {
		props.Mode = *d.desiredProperties.Mode
	}
	if d.desiredProperties.Owner != nil {
		props.Owner = *d.desiredProperties.Owner
	}
	if d.desiredProperties.Group != nil {
		props.Group = *d.desiredProperties.Group
	}

	params := ops_directories.NewPutDirectoryParamsWithContext(ctx)
	params.Path = d.path
	params.Properties = props

	// Existing directory, enforce ETag
	if d.currentProperties != nil && d.etag != "" {
		params.SetIfMatch(pointer.To(d.etag))
	}

	created, noContent, err := d.cfg.Client.Directories.PutDirectory(params)
	if err != nil {
		if payload := getErrorPayload(err); payload != nil {
			return &APIError{Code: payload.Code, Message: payload.Message}
		}

		return fmt.Errorf("failed to apply file: %w", err)
	}

	switch {
	case created != nil:
		d.lastOperation = OperationCreate
		d.etag = created.ETag
	case noContent != nil:
		d.lastOperation = OperationUpdate
		d.etag = noContent.ETag
	default:
		return fmt.Errorf("unexpected nil response")
	}

	return nil
}

func (d *Directory) Backup(ctx context.Context) (bool, error) {
	// Only backup if directory exists
	if d.currentState != StatePresent {
		return false, nil
	}

	// If desired state is absent, backup content for full restore
	if d.desiredState == StateAbsent {
		return d.backup(ctx)
	}

	// If desired state is present and properties are changing, backup current properties
	// (f.currentProperties is already stored).
	return false, nil
}

func (d *Directory) backup(ctx context.Context) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(d.backupPath()), 0755); err != nil {
		return false, err
	}

	fd, err := os.Create(d.backupPath())
	if err != nil {
		return false, err
	}
	defer fd.Close()

	params := ops_content.NewDownloadParamsWithContext(ctx)
	params.Path = d.path
	params.Recursive = pointer.To(true)

	_, err = d.cfg.Client.Content.Download(params, fd)
	if err != nil {
		// Clean up backup file on error
		os.Remove(d.backupPath())

		if payload := getErrorPayload(err); payload != nil {
			return false, &APIError{Code: payload.Code, Message: payload.Message}
		}

		return false, fmt.Errorf("failed to backup directory: %w", err)
	}

	return true, nil
}

func (d *Directory) Rollback(ctx context.Context) error {
	switch d.lastOperation {
	case OperationNone:
		return nil
	case OperationCreate:
		params := ops_directories.NewDeleteDirectoryParamsWithContext(ctx)
		params.Path = d.path
		params.SetIfMatch(pointer.To(d.etag))

		_, err := d.cfg.Client.Directories.DeleteDirectory(params)
		if err != nil {
			if payload := getErrorPayload(err); payload != nil {
				return &APIError{Code: payload.Code, Message: payload.Message}
			}

			return fmt.Errorf("failed to delete file: %w", err)
		}
		return err
	case OperationUpdate:
		return d.rollbackProperties(ctx)
	case OperationDelete:
		return d.restoreFromBackup(ctx)
	}

	return nil
}

func (d *Directory) rollbackProperties(ctx context.Context) error {
	if d.currentProperties == nil {
		return fmt.Errorf("no properties to rollback")
	}

	props := &models.DirectoryProperties{
		Mode:  d.currentProperties.Mode,
		Owner: d.currentProperties.Owner,
		Group: d.currentProperties.Group,
	}

	params := ops_directories.NewPutDirectoryParamsWithContext(ctx)
	params.Path = d.path
	params.Properties = props

	if d.etag != "" {
		params.SetIfMatch(pointer.To(d.etag))
	}

	_, _, err := d.cfg.Client.Directories.PutDirectory(params)
	if err != nil {
		if payload := getErrorPayload(err); payload != nil {
			return &APIError{Code: payload.Code, Message: payload.Message}
		}

		return fmt.Errorf("failed to put file: %w", err)
	}

	return nil
}

func (d *Directory) backupPath() string {
	safe := strings.ReplaceAll(strings.TrimPrefix(d.path, "/"), "/", "-")
	return filepath.Join(d.cfg.BackupDir, safe+"-dir.tar.gz")
}

func (d *Directory) restoreFromBackup(ctx context.Context) error {
	// Check if backup file exists
	if _, err := os.Stat(d.backupPath()); os.IsNotExist(err) {
		return fmt.Errorf("no backup file found at %s", d.backupPath())
	}

	fd, err := os.Open(d.backupPath())
	if err != nil {
		return fmt.Errorf("failed to open backup: %w", err)
	}
	defer fd.Close()

	params := ops_content.NewUploadParamsWithContext(ctx)
	params.Path = d.path
	params.Recursive = pointer.To(true)
	params.Content = fd

	_, _, err = d.cfg.Client.Content.Upload(params)
	if err != nil {
		if payload := getErrorPayload(err); payload != nil {
			return &APIError{Code: payload.Code, Message: payload.Message}
		}
		return fmt.Errorf("failed to restore directory from backup: %w", err)
	}

	// TODO: Clean up backup file after successful restore

	return nil
}
