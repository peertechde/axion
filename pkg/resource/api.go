package resource

import (
	"errors"
	"fmt"

	ops_directories "peertech.de/axion/api/client/directories"
	ops_files "peertech.de/axion/api/client/files"
	"peertech.de/axion/api/models"
)

type APIError struct {
	Code    int64
	Message string
}

func (ae *APIError) Error() string {
	return fmt.Sprintf("Error code %d: %s", ae.Code, ae.Message)
}

func fileNotFound(err error) bool {
	var notFound *ops_files.GetFilePropertiesNotFound
	return errors.As(err, &notFound)
}

func directoryNotFound(err error) bool {
	var notFound *ops_directories.GetDirectoryPropertiesNotFound
	return errors.As(err, &notFound)
}

type errorWithPayload interface {
	GetPayload() *models.Error
}

func getErrorPayload(err error) *models.Error {
	var e errorWithPayload
	if errors.As(err, &e) {
		return e.GetPayload()
	}
	return nil
}
